package relay

import "evergate/internal/store"

// InvalidateConfig 使渠道与降级规则快照失效；管理台写操作成功后调用，下一次请求重建。
func (r *Relay) InvalidateConfig() {
	r.cfgMu.Lock()
	r.channels, r.channelsRead = nil, false
	r.fallbacks, r.fallbackRead = nil, false
	r.cfgMu.Unlock()
}

// enabledChannels 返回启用渠道快照：热路径只读内存，首次访问或失效后回源一次。
func (r *Relay) enabledChannels() ([]*store.Channel, error) {
	if r.opt.DB == nil {
		return nil, nil
	}
	r.cfgMu.RLock()
	channels, loaded := r.channels, r.channelsRead
	r.cfgMu.RUnlock()
	if loaded {
		return channels, nil
	}

	r.cfgMu.Lock()
	defer r.cfgMu.Unlock()
	if r.channelsRead {
		return r.channels, nil
	}
	var fresh []*store.Channel
	if err := r.opt.DB.Where("enabled = ?", true).Find(&fresh).Error; err != nil {
		return nil, err
	}
	r.channels, r.channelsRead = fresh, true
	return fresh, nil
}

// fallbackRule 返回模型降级规则快照；未配置该模型时 found=false。
func (r *Relay) fallbackRule(model string) (store.ModelFallback, bool, error) {
	if r.opt.DB == nil {
		return store.ModelFallback{}, false, nil
	}
	r.cfgMu.RLock()
	rules, loaded := r.fallbacks, r.fallbackRead
	r.cfgMu.RUnlock()
	if !loaded {
		r.cfgMu.Lock()
		if !r.fallbackRead {
			var fresh []store.ModelFallback
			if err := r.opt.DB.Find(&fresh).Error; err != nil {
				r.cfgMu.Unlock()
				return store.ModelFallback{}, false, err
			}
			r.fallbacks, r.fallbackRead = fresh, true
		}
		rules = r.fallbacks
		r.cfgMu.Unlock()
	}
	for i := range rules {
		if rules[i].Model == model {
			return rules[i], true, nil
		}
	}
	return store.ModelFallback{}, false, nil
}
