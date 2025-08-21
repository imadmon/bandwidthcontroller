package bandwidthcontroller

import "context"

type Option func(*BandwidthController)

func WithContext(ctx context.Context) Option {
	return func(bc *BandwidthController) {
		bc.ctx = ctx
	}
}

func WithConfig(cfg Config) Option {
	return func(bc *BandwidthController) {
		bc.cfg = defaultConfig()

		if cfg.BandwidthUpdaterInterval != nil {
			bc.cfg.BandwidthUpdaterInterval = cfg.BandwidthUpdaterInterval
		}

		if cfg.MinFileBandwidthPercentage != nil {
			bc.cfg.MinFileBandwidthPercentage = cfg.MinFileBandwidthPercentage
		}
	}
}
