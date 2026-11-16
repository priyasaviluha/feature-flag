package environment

import "context"

type Store interface {
	List(context.Context) ([]Environment, error)
	FindByID(context.Context, string) (Environment, error)
	UpdateName(context.Context, string, string) (Environment, error)
	Archive(context.Context, string) error
}

type Service struct {
	repository Store
}

func NewService(repository Store) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]Environment, error) {
	return s.repository.List(ctx)
}

func (s *Service) FindByID(ctx context.Context, id string) (Environment, error) {
	return s.repository.FindByID(ctx, id)
}

func (s *Service) UpdateName(ctx context.Context, id, name string) (Environment, error) {
	return s.repository.UpdateName(ctx, id, name)
}

func (s *Service) Archive(ctx context.Context, id string) error {
	return s.repository.Archive(ctx, id)
}
