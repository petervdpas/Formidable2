package app

import (
	"errors"

	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/history"
	"github.com/petervdpas/formidable2/internal/modules/nav"
)

// navReplayAdapter wraps *nav.Manager so it satisfies history.Navigator.
// nav returns (*Result, error); we collapse Success=false into the
// error channel so history.Service has one failure shape.
type navReplayAdapter struct{ m *nav.Manager }

func (a *navReplayAdapter) NavigateToFormidable(href string) error {
	if a == nil || a.m == nil {
		return nil
	}
	res, err := a.m.NavigateToFormidable(href)
	if err != nil {
		return err
	}
	if res != nil && !res.Success {
		return errors.New(res.Error)
	}
	return nil
}

// historyPersistAdapter writes history snapshots back into user.json
// when cfg.history.persist is on. Reading the live config each call
// keeps history.Service oblivious to the persist toggle - flipping
// the setting takes effect on the next mutation without a restart.
type historyPersistAdapter struct{ cfg *config.Manager }

func (a *historyPersistAdapter) PersistSnapshot(s history.Snapshot) {
	if a == nil || a.cfg == nil {
		return
	}
	cur, err := a.cfg.LoadUserConfig()
	if err != nil || !cur.History.Persist {
		return
	}
	_, _ = a.cfg.UpdateUserConfig(map[string]any{
		"history": config.History{
			Enabled: cur.History.Enabled,
			Persist: true,
			MaxSize: cur.History.MaxSize,
			Stack:   s.Stack,
			Index:   s.Index,
		},
	})
}
