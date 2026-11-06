package integrations

import "context"

type emailMode interface {
	Mode(context.Context) (string, error)
}

func (s *Service) emailEnabled(ctx context.Context) (bool, error) {
	if dynamic, ok := s.adapters.Email.(emailMode); ok {
		mode, err := dynamic.Mode(ctx)
		return mode != "noop" && mode != "", err
	}
	return s.adapters.Email != nil && s.adapters.Capabilities.Email != "noop", nil
}
