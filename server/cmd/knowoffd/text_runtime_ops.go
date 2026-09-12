package main

import (
	"context"

	"github.com/knowoff/knowoff/server/internal/admin"
)

func (r *textRuntime) adminRuntimeHooks() admin.RuntimeHooks {
	token := r.Owner.Token()
	return admin.RuntimeHooks{
		OwnerID: token.IncarnationID, Generation: token.Generation,
		BeginDrain: r.Lobby.BeginDrain,
		Status: func(ctx context.Context) (admin.RuntimeStatus, error) {
			var status admin.RuntimeStatus
			var err error
			if status.Process, err = r.Lobby.DrainStatus(ctx); err != nil {
				return status, err
			}
			if status.Durable, err = r.Values.DrainStatus(ctx); err != nil {
				return status, err
			}
			// A lease loss during the durable read invalidates this process's counts.
			return status, r.Owner.Check(ctx)
		},
	}
}
