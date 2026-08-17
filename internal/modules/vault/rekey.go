package vault

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Staging directories used by a re-key. Dot-prefixed so List and the loaders
// ignore them if a crash ever leaves one behind.
const (
	rekeyStageDir  = ".rekey.tmp"
	rekeyBackupDir = ".rekey.bak"
)

// ChangePassword re-encrypts the whole vault under a new master password.
//
// The old password is required even though the vault is already unlocked: an
// unlocked app left on a desk should not be enough to lock its owner out.
//
// Every record is carried across, including files this package did not write.
// A record that cannot be read aborts the whole operation rather than being
// dropped, because a silent partial re-key is indistinguishable from data loss
// until the moment someone needs the missing secret.
//
// The new vault is built in a staging directory and swapped in, so a crash
// leaves either the old vault or the new one, never a mix of records under two
// different keys.
func (v *Vault) ChangePassword(oldPassword, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("vault: new master password is empty")
	}

	v.mu.Lock()
	key, head := v.key, v.head
	v.mu.Unlock()
	if key == nil {
		return ErrLocked
	}

	if err := v.verifyPassword(oldPassword, head); err != nil {
		return err
	}

	// Read everything before touching disk, so an unreadable record costs
	// nothing but an error.
	plain, strays, err := v.snapshot(key, head.VaultID)
	if err != nil {
		return err
	}

	newHead, records, newKey, err := reseal(head, plain, newPassword, v.params)
	if err != nil {
		return err
	}

	if err := v.swapIn(newHead, records, strays); err != nil {
		zero(newKey)
		return err
	}

	v.mu.Lock()
	zero(v.key)
	v.key = newKey
	v.head = newHead
	v.mu.Unlock()
	v.touch()
	return nil
}

// verifyPassword re-derives from the stored salt and checks the canary, so a
// wrong old password fails before anything is written.
func (v *Vault) verifyPassword(password string, head header) error {
	salt, err := base64.StdEncoding.DecodeString(head.KDF.Salt)
	if err != nil || len(salt) != saltLen {
		return corrupt(v.headerPath(), "salt is not a valid 16-byte value", err)
	}
	probe := deriveKey(password, salt, Params{
		MemoryKiB:   head.KDF.MemoryKiB,
		Iterations:  head.KDF.Iterations,
		Parallelism: head.KDF.Parallelism,
	})
	defer zero(probe)

	_, ok, err := open(probe, head.VaultID, canarySlot, head.Canary, v.headerPath())
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidPassword
	}
	return nil
}

// snapshot decrypts every record, and lists the files in secrets/ that are not
// records so they can be carried across untouched.
func (v *Vault) snapshot(key []byte, vaultID string) (map[string]string, []string, error) {
	dir := filepath.Join(v.dir, secretsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil, nil
		}
		return nil, nil, err
	}

	plain := map[string]string{}
	var strays []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), secretExt) {
			strays = append(strays, e.Name())
			continue
		}
		name := strings.TrimSuffix(e.Name(), secretExt)
		path := filepath.Join(dir, e.Name())

		var rec record
		if err := readJSON(path, &rec); err != nil {
			return nil, nil, fmt.Errorf("vault: cannot re-key, record %q is unreadable: %w", name, err)
		}
		text, ok, err := open(key, vaultID, name, rec, path)
		if err != nil {
			return nil, nil, fmt.Errorf("vault: cannot re-key, record %q is unreadable: %w", name, err)
		}
		if !ok {
			return nil, nil, fmt.Errorf("vault: cannot re-key, record %q does not decrypt with the current key", name)
		}
		plain[name] = string(text)
	}
	return plain, strays, nil
}

// reseal builds the replacement header and records under a new password. The
// vault id is deliberately preserved: it is bound into every record's AAD, and
// changing it would invalidate anything not rewritten here.
func reseal(head header, plain map[string]string, newPassword string, params Params) (header, map[string]record, []byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return header{}, nil, nil, err
	}

	// Re-key at the parameters the vault already uses, unless it predates them
	// being recorded, so a re-key never silently weakens the work factor.
	next := Params{
		MemoryKiB:   head.KDF.MemoryKiB,
		Iterations:  head.KDF.Iterations,
		Parallelism: head.KDF.Parallelism,
	}
	if next.MemoryKiB == 0 || next.Iterations == 0 {
		next = params
	}

	newKey := deriveKey(newPassword, salt, next)
	canary, err := seal(newKey, head.VaultID, canarySlot, canaryPlaintext)
	if err != nil {
		zero(newKey)
		return header{}, nil, nil, err
	}

	records := make(map[string]record, len(plain))
	now := utcTime{time.Now().UTC()}
	for name, text := range plain {
		rec, err := seal(newKey, head.VaultID, name, []byte(text))
		if err != nil {
			zero(newKey)
			return header{}, nil, nil, err
		}
		rec.Version = formatVersion
		rec.Algorithm = algorithmAESGCM
		rec.UpdatedUTC = now
		records[name] = rec
	}

	newHead := head
	newHead.Version = formatVersion
	newHead.KDF = kdfHeader{
		Algorithm:   algorithmArgon2id,
		MemoryKiB:   next.MemoryKiB,
		Iterations:  next.Iterations,
		Parallelism: next.Parallelism,
		Salt:        base64.StdEncoding.EncodeToString(salt),
	}
	newHead.Canary = canary
	return newHead, records, newKey, nil
}

// swapIn stages the new vault beside the old one, then moves the old aside and
// the new into place. The window where neither is present is two renames wide,
// and the backup survives any failure inside it.
func (v *Vault) swapIn(head header, records map[string]record, strays []string) error {
	stage := filepath.Join(v.dir, rekeyStageDir)
	backup := filepath.Join(v.dir, rekeyBackupDir)
	_ = os.RemoveAll(stage)
	_ = os.RemoveAll(backup)

	stageSecrets := filepath.Join(stage, secretsDirName)
	if err := os.MkdirAll(stageSecrets, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	if err := writeJSON(filepath.Join(stage, headerFileName), head); err != nil {
		return err
	}
	for name, rec := range records {
		if err := writeJSON(filepath.Join(stageSecrets, name+secretExt), rec); err != nil {
			return err
		}
	}
	// Files this package did not write are copied verbatim. Dropping them
	// would quietly delete another tool's data from a shared vault.
	for _, name := range strays {
		raw, err := os.ReadFile(filepath.Join(v.dir, secretsDirName, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stageSecrets, name), raw, 0o600); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(backup, 0o700); err != nil {
		return err
	}
	liveHeader := v.headerPath()
	liveSecrets := filepath.Join(v.dir, secretsDirName)

	if err := os.Rename(liveSecrets, filepath.Join(backup, secretsDirName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(liveHeader, filepath.Join(backup, headerFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		v.restore(backup)
		return err
	}

	if err := os.Rename(stageSecrets, liveSecrets); err != nil {
		v.restore(backup)
		return err
	}
	if err := os.Rename(filepath.Join(stage, headerFileName), liveHeader); err != nil {
		_ = os.RemoveAll(liveSecrets)
		v.restore(backup)
		return err
	}

	return os.RemoveAll(backup)
}

// restore puts the pre-swap vault back after a failed swap. Best effort: if
// this fails too, the backup directory is still on disk for a human to move.
func (v *Vault) restore(backup string) {
	_ = os.Rename(filepath.Join(backup, secretsDirName), filepath.Join(v.dir, secretsDirName))
	_ = os.Rename(filepath.Join(backup, headerFileName), v.headerPath())
	_ = os.Remove(backup)
}
