package bandwidthcontroller

import "context"

type Option func(*BandwidthController)

func WithContext(ctx context.Context) Option {
	return func(bc *BandwidthController) {
		bc.ctx = ctx
	}
}

// WithConfig merges the provided config over defaults (partial overrides supported).
func WithConfig(cfg Config) Option {
	return func(bc *BandwidthController) {
		bc.cfg = defaultConfig()

		if cfg.BandwidthUpdaterInterval != nil {
			bc.cfg.BandwidthUpdaterInterval = cfg.BandwidthUpdaterInterval
		}

		if cfg.MinGroupBandwidthPercentage != nil {
			for g, v := range cfg.MinGroupBandwidthPercentage {
				bc.cfg.MinGroupBandwidthPercentage[g] = v
			}
		}

		if cfg.MinStreamBandwidthInBytes != nil {
			for g, v := range cfg.MinStreamBandwidthInBytes {
				bc.cfg.MinStreamBandwidthInBytes[g] = v
			}
		}
	}
}
