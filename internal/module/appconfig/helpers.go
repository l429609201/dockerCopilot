package appconfig

// maskSecret 对敏感字符串脱敏：保留前后各若干位，中间用星号，短串全星号。
func maskSecret(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return ""
	}
	if n <= 4 {
		return "****"
	}
	head := 2
	tail := 2
	return string(runes[:head]) + "****" + string(runes[n-tail:])
}

// ListRegistries 返回全部凭据的副本切片，供自适应匹配等场景遍历使用。
func (s *Store) ListRegistries() []RegistryCredential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RegistryCredential, len(s.cfg.Registries))
	copy(out, s.cfg.Registries)
	return out
}


// FindRegistry 按ID查找凭据，返回副本与是否存在。
func (s *Store) FindRegistry(id string) (RegistryCredential, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.cfg.Registries {
		if r.ID == id {
			return r, true
		}
	}
	return RegistryCredential{}, false
}

// FindScheduledRule 按ID查找定时规则副本。
func (s *Store) FindScheduledRule(id string) (ScheduledUpdateRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.cfg.ScheduledUpdates {
		if r.ID == id {
			return r, true
		}
	}
	return ScheduledUpdateRule{}, false
}

// MaskedRegistries 返回脱敏后的凭据列表，供接口回显，绝不含明文密码。
func (s *Store) MaskedRegistries() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]map[string]interface{}, 0, len(s.cfg.Registries))
	for _, r := range s.cfg.Registries {
		result = append(result, map[string]interface{}{
			"id":       r.ID,
			"name":     r.Name,
			"registry": r.Registry,
			"username": r.Username,
			"password": maskSecret(r.Password),
		})
	}
	return result
}

// RawTelegramToken 返回明文 Bot Token。
// 仅供需要"可查看明文"的场景使用（如混淆后回显给已登录用户），调用方需自行保证安全。
func (s *Store) RawTelegramToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Telegram.Token
}

// MaskedTelegram 返回脱敏后的 Telegram 配置。
func (s *Store) MaskedTelegram() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.cfg.Telegram
	return map[string]interface{}{
		"enabled":         t.Enabled,
		"token":           maskSecret(t.Token),
		"allowedChatIds":  t.AllowedChatIDs,
		"proxy":           t.Proxy,
		"pollIntervalSec": t.PollIntervalSec,
		"notifyUpdate":    t.NotifyUpdate,
	}
}
