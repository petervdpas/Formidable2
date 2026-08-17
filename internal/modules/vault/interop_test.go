package vault

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// This file proves the compatibility claim in the package doc rather than
// asserting it. It drives the real SecretBlast build through a throwaway
// console host and round-trips a vault in both directions: a vault written by
// .NET is read by Go, and a vault written by Go is read by .NET.
//
// The test skips when the SDK or the SecretBlast sources are absent, and under
// -short, because a restore plus build is slow. It is not skipped silently:
// the reason is always logged.

const secretBlastProject = "/home/peter/Projects/SecretBlast/SecretBlast/SecretBlast.csproj"

// interopProgram is a minimal host over ISecretVault. Work factors are passed
// in so the harness matches the Go side's fast test parameters.
const interopProgram = `
using System;
using System.Threading.Tasks;
using SecretBlast;

internal static class Program
{
    private static VaultOptions Opts() => new()
    {
        AutoLockIdle = TimeSpan.Zero,
        Kdf = new Argon2Parameters(1024, 1, 1),
    };

    private static async Task<int> Main(string[] args)
    {
        var verb = args[0];
        var dir  = args[1];
        var pw   = args[2];

        switch (verb)
        {
            case "create":
            {
                using var v = SecretVault.Create(dir, pw, Opts());
                for (var i = 3; i + 1 < args.Length; i += 2)
                    await v.SetAsync(args[i], args[i + 1]);
                return 0;
            }
            case "get":
            {
                using var v = SecretVault.Open(dir, Opts());
                await v.UnlockAsync(pw);
                Console.Out.Write(await v.GetAsync(args[3]));
                return 0;
            }
            case "set":
            {
                using var v = SecretVault.Open(dir, Opts());
                await v.UnlockAsync(pw);
                await v.SetAsync(args[3], args[4]);
                return 0;
            }
            case "list":
            {
                using var v = SecretVault.Open(dir, Opts());
                await v.UnlockAsync(pw);
                Console.Out.Write(string.Join(",", await v.ListAsync()));
                return 0;
            }
            case "unlock":
            {
                using var v = SecretVault.Open(dir, Opts());
                try
                {
                    await v.UnlockAsync(pw);
                    Console.Out.Write("ok");
                }
                catch (InvalidMasterPasswordException)
                {
                    Console.Out.Write("bad-password");
                }
                return 0;
            }
            default:
                Console.Error.Write("unknown verb");
                return 2;
        }
    }
}
`

const interopProject = `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net10.0</TargetFramework>
    <Nullable>disable</Nullable>
    <ImplicitUsings>disable</ImplicitUsings>
    <AssemblyName>vaultinterop</AssemblyName>
    <RootNamespace>vaultinterop</RootNamespace>
    <GenerateDocumentationFile>false</GenerateDocumentationFile>
    <InvariantGlobalization>true</InvariantGlobalization>
  </PropertyGroup>
  <ItemGroup>
    <ProjectReference Include="SECRETBLAST_CSPROJ" />
  </ItemGroup>
</Project>
`

// dotnetHarness builds the console host once and returns a runner for it.
func dotnetHarness(t *testing.T) func(t *testing.T, args ...string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("interop: skipped under -short (a dotnet restore and build is slow)")
	}
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip("interop: the dotnet SDK is not on PATH")
	}
	if _, err := os.Stat(secretBlastProject); err != nil {
		t.Skipf("interop: SecretBlast sources not found at %s", secretBlastProject)
	}

	proj := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(proj, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("Program.cs", interopProgram)
	write("vaultinterop.csproj", strings.Replace(interopProject, "SECRETBLAST_CSPROJ", secretBlastProject, 1))

	build := exec.Command("dotnet", "build", "-c", "Release", "--nologo", "-v", "quiet")
	build.Dir = proj
	build.Env = append(os.Environ(), "DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("interop: cannot build the harness (offline restore?): %v\n%s", err, out)
	}

	// Invoke the built binary rather than `dotnet run`: the run driver claims
	// options for itself and the forwarding rules have moved between SDKs.
	// Executing the apphost keeps argv exactly as written here.
	exe := filepath.Join(proj, "bin", "Release", "net10.0", "vaultinterop")
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("interop: harness binary not found at %s: %v", exe, err)
	}

	return func(t *testing.T, args ...string) string {
		t.Helper()
		cmd := exec.Command(exe, args...)
		cmd.Dir = proj
		cmd.Env = append(os.Environ(), "DOTNET_CLI_TELEMETRY_OPTOUT=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("harness %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}
}

// openInterop opens a vault with the fast parameters the harness uses.
func openInterop(t *testing.T, dir, password string) *Vault {
	t.Helper()
	v, err := Open(dir, WithAutoLock(0))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := v.Unlock(password); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	t.Cleanup(v.Lock)
	return v
}

func TestInterop_GoReadsAVaultWrittenByDotNet(t *testing.T) {
	run := dotnetHarness(t)
	dir := filepath.Join(t.TempDir(), "vault")
	const pw = "correct horse battery staple"

	run(t, "create", dir, pw,
		"github-token", "ghp_from_dotnet",
		"azure-prod-sql", "Server=tcp:example;Password=p@ss;",
		"unicode.secret", "wachtwoord-één-\U0001F510")

	v := openInterop(t, dir, pw)

	names, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "azure-prod-sql,github-token,unicode.secret" {
		t.Fatalf("names = %v", names)
	}

	want := map[string]string{
		"github-token":   "ghp_from_dotnet",
		"azure-prod-sql": "Server=tcp:example;Password=p@ss;",
		"unicode.secret": "wachtwoord-één-\U0001F510",
	}
	for name, expect := range want {
		got, err := v.Get(name)
		if err != nil {
			t.Errorf("Get(%q): %v", name, err)
			continue
		}
		if got != expect {
			t.Errorf("Get(%q) = %q, want %q", name, got, expect)
		}
	}
}

func TestInterop_DotNetReadsAVaultWrittenByGo(t *testing.T) {
	run := dotnetHarness(t)
	dir := filepath.Join(t.TempDir(), "vault")
	const pw = "another master password"

	v, err := Create(dir, pw, WithParams(fastParams), WithAutoLock(0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Lock)
	if err := v.Set("api-client-northwind", "odata-bearer-token"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set("empty-one", ""); err != nil {
		t.Fatal(err)
	}

	if got := run(t, "get", dir, pw, "api-client-northwind"); got != "odata-bearer-token" {
		t.Fatalf("dotnet read %q", got)
	}
	if got := run(t, "get", dir, pw, "empty-one"); got != "" {
		t.Fatalf("dotnet read %q for the empty secret", got)
	}
	// SecretBlast returns directory order; this package sorts. Compare as a
	// set: ordering is a per-implementation choice, not part of the format.
	listed := strings.Split(run(t, "list", dir, pw), ",")
	slices.Sort(listed)
	if strings.Join(listed, ",") != "api-client-northwind,empty-one" {
		t.Fatalf("dotnet listed %v", listed)
	}
}

func TestInterop_BothRuntimesWriteIntoOneVault(t *testing.T) {
	run := dotnetHarness(t)
	dir := filepath.Join(t.TempDir(), "vault")
	const pw = "shared vault password"

	run(t, "create", dir, pw, "from-dotnet", "value-a")

	v := openInterop(t, dir, pw)
	if err := v.Set("from-go", "value-b"); err != nil {
		t.Fatal(err)
	}
	// Overwrite a record .NET wrote, so the rewrite path is covered too.
	if err := v.Set("from-dotnet", "rewritten-by-go"); err != nil {
		t.Fatal(err)
	}

	if got := run(t, "get", dir, pw, "from-go"); got != "value-b" {
		t.Errorf("dotnet could not read the Go-written record: %q", got)
	}
	if got := run(t, "get", dir, pw, "from-dotnet"); got != "rewritten-by-go" {
		t.Errorf("dotnet read a stale value after a Go rewrite: %q", got)
	}

	run(t, "set", dir, pw, "from-go", "rewritten-by-dotnet")
	got, err := v.Get("from-go")
	if err != nil {
		t.Fatal(err)
	}
	if got != "rewritten-by-dotnet" {
		t.Errorf("go read %q after a dotnet rewrite", got)
	}
}

func TestInterop_WrongPasswordAgreesAcrossRuntimes(t *testing.T) {
	run := dotnetHarness(t)
	dir := filepath.Join(t.TempDir(), "vault")

	v, err := Create(dir, "the right one", WithParams(fastParams), WithAutoLock(0))
	if err != nil {
		t.Fatal(err)
	}
	v.Lock()

	if got := run(t, "unlock", dir, "the wrong one"); got != "bad-password" {
		t.Errorf("dotnet said %q for a wrong password", got)
	}
	if got := run(t, "unlock", dir, "the right one"); got != "ok" {
		t.Errorf("dotnet said %q for the right password", got)
	}
}

func TestInterop_DefaultParamsAreCompatible(t *testing.T) {
	// The fast parameters above prove the format; this proves the shipped work
	// factors match too, so a real vault opens on either side.
	if testing.Short() {
		t.Skip("skipped under -short: default Argon2 parameters are deliberately slow")
	}
	dir := t.TempDir()
	start := time.Now()
	v, err := Create(dir, "pw", WithParams(DefaultParams), WithAutoLock(0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Lock)
	t.Logf("Argon2id at m=%d t=%d p=%d took %s",
		DefaultParams.MemoryKiB, DefaultParams.Iterations, DefaultParams.Parallelism, time.Since(start))

	if err := v.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, WithAutoLock(0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Lock)
	if err := reopened.Unlock("pw"); err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Get("k"); got != "v" {
		t.Fatalf("value = %q", got)
	}
}
