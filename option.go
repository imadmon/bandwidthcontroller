package bandwidthcontroller

import "context"

type Option func(*BandwidthController)

func WithContext(ctx context.Context) Option {
	return func(bc *BandwidthController) {
		bc.ctx = ctx
	}
}

// WithConfig merges the provided config over defaults (partial overrides supported).
func WithConfig(cfg ControllerConfig) Option {
	return func(bc *BandwidthController) {
		bc.cfg = defaultConfig()

		if cfg.SchedulerInterval != nil {
			bc.cfg.SchedulerInterval = cfg.SchedulerInterval
		}

		if cfg.StreamIdleTimeout != nil {
			bc.cfg.StreamIdleTimeout = cfg.StreamIdleTimeout
		}

		if cfg.MinGroupBandwidthPercentShare != nil {
			for g, v := range cfg.MinGroupBandwidthPercentShare {
				bc.cfg.MinGroupBandwidthPercentShare[g] = v
			}
		}

		if cfg.MinStreamBandwidthInBytesPerSec != nil {
			for g, v := range cfg.MinStreamBandwidthInBytesPerSec {
				bc.cfg.MinStreamBandwidthInBytesPerSec[g] = v
			}
		}
	}
}
