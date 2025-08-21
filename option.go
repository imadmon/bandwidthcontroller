package bandwidthcontroller

import "context"

type Option func(*BandwidthController)

func WithContext(ctx context.Context) Option {
	return func(bc *BandwidthController) {
		bc.ctx = ctx
	}
}
