package device

import (
	"context"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
)

func (w *Worker) dataIdentityGeneration() uint64 {
	w.cacheMu.RLock()
	defer w.cacheMu.RUnlock()
	return w.state.Identity.Generation
}

// retryNetwork reconnects only the current worker and SIM under the current
// card policy. Explicit StopNetwork and switch begin revoke the old intent.
func (w *Worker) retryNetwork(ctx context.Context, generation uint64) (bool, error) {
	if ctx.Err() != nil || !w.networkDesired.Load() ||
		generation != w.dataIdentityGeneration() || !w.autoConnectDataAllowed() ||
		w.cellularRegistrationBlocked() || w.Pool == nil || w.Pool.GetWorker(w.ID) != w {
		return true, nil
	}
	nc := w.NetworkController()
	if nc == nil || nc.IsConnected() {
		return true, nil
	}
	err := w.startNetwork(ctx)
	if err == nil && nc.IsConnected() && w.autoConnectDataAllowed() && w.networkDesired.Load() {
		w.Pool.refreshIPs(w, true)
	}
	return err == nil, err
}

func (w *Worker) scheduleNetworkReconnect() {
	if w == nil || w.Pool == nil || w.Pool.ctx.Err() != nil || w.Pool.GetWorker(w.ID) != w || !w.networkDesired.Load() ||
		!w.autoConnectDataAllowed() || w.cellularRegistrationBlocked() ||
		!w.networkReconnectRunning.CompareAndSwap(false, true) {
		return
	}
	generation := w.dataIdentityGeneration()
	go func() {
		defer func() {
			w.networkReconnectRunning.Store(false)
			// A new SIM may have requested data while the old retry was
			// still waiting. Start its own loop, never reuse the old one.
			if generation != w.dataIdentityGeneration() {
				w.scheduleNetworkReconnect()
			}
		}()
		for delay := 5 * time.Second; ; delay = min(delay*2, time.Minute) {
			timer := time.NewTimer(delay)
			select {
			case <-w.Pool.ctx.Done():
				timer.Stop()
				return
			case <-w.stop:
				timer.Stop()
				return
			case <-timer.C:
			}
			done, err := w.retryNetwork(w.Pool.ctx, generation)
			if done {
				return
			}
			logger.Debug("蜂窝数据重连失败，等待退避后按当前卡策略重试", "device", w.ID, "err", err)
		}
	}()
}
