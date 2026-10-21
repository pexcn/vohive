package backend

import "context"

// radioGuard preserves optional backend capabilities while letting the host
// serialize radio activation with card-policy changes. Bind before first use.
type radioGuard struct {
	beforeOnline func(context.Context) (func(), error)
}

func (g *radioGuard) SetRadioOnlineGuard(guard func(context.Context) (func(), error)) {
	g.beforeOnline = guard
}

func (g *radioGuard) beginOnline(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if g.beforeOnline != nil {
		return g.beforeOnline(ctx)
	}
	return func() {}, ctx.Err()
}
