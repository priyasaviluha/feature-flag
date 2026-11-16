package environment

import "context"

type Store interface {
	List(context.Context) ([]Environment, error)
	FindByID(context.Context, string) (Environment, error)
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
