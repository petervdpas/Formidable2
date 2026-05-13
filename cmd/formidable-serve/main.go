// Command formidable-serve is the directive stub for Formidable's
// planned server-mode entrypoint. It does not yet do anything — running
// it prints a one-line marker and exits non-zero.
//
// Why this file exists as a real binary rather than a doc:
//
//   - The Desktop / Server split lives in internal/modules/auth (Mode
//     constants, SubscriptionResolver, ProfileProvider, etc.) but the
//     desktop app currently pins itself to ModeDesktop. The existence
//     of this binary, sitting next to the desktop entrypoint, documents
//     that the divergence is real and intentional rather than
//     theoretical.
//   - When the CLI mode is built out, it lands here. There will be no
//     "where should I put this?" decision.
//   - go build ./cmd/formidable-serve already works, which keeps the
//     trust-boundary scaffold honest: anyone who removes the auth
//     module's Server-side surface (Subscription, SubscriptionResolver,
//     Mode) will break this build target on the next CI run.
//
// See design discussions in Formidable2's followups + the auth module.
package main

import (
	"fmt"
	"os"

	"github.com/petervdpas/formidable2/internal/modules/auth"
)

const notImplementedMessage = `formidable-serve: not yet implemented — server mode is planned.

The directive scaffold lives in internal/modules/auth:
  - auth.Mode (Desktop / Server)
  - auth.Subscription (capability grant: profile × templates × methods)
  - auth.SubscriptionResolver (HTTP request → Identity, currently
    returns ErrNotImplemented)
  - auth.ProfileProvider, auth.DesktopResolver
  - auth.LoopbackOnly / RequireOrigin / ResolveIdentity middleware

When this binary is built out it will:
  1. Load a config + profile from disk (no GUI),
  2. Bind a configurable interface (not just 127.0.0.1),
  3. Mount the api handler behind ResolveIdentity backed by a
     SubscriptionResolver wired to a real SubscriptionStore,
  4. Attribute every write to the calling subscription's profile
     via auth.WithIdentity on the request context.

Until then this is a non-functional placeholder that exists so the
direction is plain in code, not just in design docs.
`

func main() {
	// Touch each scaffold symbol so a future refactor that drops one
	// breaks this build target — the CLI stub's job is to keep the
	// directive types load-bearing.
	_ = auth.ModeServer
	_ = auth.SubscriptionResolver{}
	_ = auth.Subscription{}

	fmt.Fprint(os.Stderr, notImplementedMessage)
	os.Exit(1)
}
