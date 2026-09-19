package lobby

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTextSanctionChecksCurrentVerifiedInstallation(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	open := func(account, hash string) *TextPeer {
		t.Helper()
		p, err := m.OpenAuthenticated(t.Context(), func(context.Context) (TextPeerBinding, error) {
			return TextPeerBinding{AccountID: account, DeviceHash: hash}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := open(uuid.NewString(), "captured")
	second := open(uuid.NewString(), "captured")
	unrelated := open(uuid.NewString(), "other")
	active := true
	check := func(_ context.Context, _ string, binding TextPeerBinding) (bool, error) {
		return active && binding.DeviceHash == "captured", nil
	}
	if err := m.EnforceSanction(t.Context(), uuid.NewString(), check); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*TextPeer{first, second} {
		select {
		case <-p.Done:
		default:
			t.Fatal("captured installation retained live peer")
		}
	}
	select {
	case <-unrelated.Done:
		t.Fatal("closed unrelated installation")
	default:
	}
	active = false
	replacement := open(first.AccountID, "captured")
	if err := m.EnforceSanction(t.Context(), uuid.NewString(), check); err != nil {
		t.Fatal(err)
	}
	select {
	case <-replacement.Done:
		t.Fatal("stale delivery closed newly allowed peer")
	default:
	}
}

func TestTextSanctionAndAuthenticatedOpenBothOrders(t *testing.T) {
	for _, openFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "delivery_first", true: "open_first"}[openFirst], func(t *testing.T) {
			m, _, _, _ := textManagerFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			var active atomic.Bool
			opened := make(chan *TextPeer, 1)
			openErr := make(chan error, 1)
			delivered := make(chan error, 1)
			verify := func(context.Context) (TextPeerBinding, error) {
				allowed := !active.Load()
				if openFirst {
					close(entered)
					<-release
				}
				if !allowed {
					return TextPeerBinding{}, errors.New("sanctioned")
				}
				return TextPeerBinding{AccountID: uuid.NewString(), DeviceHash: "captured"}, nil
			}
			check := func(context.Context, string, TextPeerBinding) (bool, error) {
				if !openFirst {
					close(entered)
					<-release
				}
				return active.Load(), nil
			}
			open := func() { p, e := m.OpenAuthenticated(ctx, verify); opened <- p; openErr <- e }
			deliver := func() { delivered <- m.EnforceSanction(ctx, uuid.NewString(), check) }
			if openFirst {
				go open()
				<-entered
				active.Store(true)
				go deliver()
				close(release)
			} else {
				if _, e := m.Open(ctx, uuid.NewString()); e != nil {
					t.Fatal(e)
				}
				active.Store(true)
				go deliver()
				<-entered
				go open()
				close(release)
			}
			p, e := <-opened, <-openErr
			if e == nil {
				select {
				case <-p.Done:
				case <-ctx.Done():
					t.Fatal("peer escaped serialized delivery")
				}
			}
			if e = <-delivered; e != nil {
				t.Fatal(e)
			}
		})
	}
}
