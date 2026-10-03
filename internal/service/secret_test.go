package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/b602op/gophkeeper/internal/domain"
)

// stubSecretRepo — управляемый мок репозитория секретов.
type stubSecretRepo struct {
	createFn          func(ctx context.Context, secret *domain.Secret) error
	findByIDFn        func(ctx context.Context, id string) (*domain.Secret, error)
	findByUserFn      func(ctx context.Context, userID string) ([]*domain.Secret, error)
	findByUserSinceFn func(ctx context.Context, userID string, since time.Time) ([]*domain.Secret, error)
	updateFn          func(ctx context.Context, secret *domain.Secret) error
	softDeleteFn      func(ctx context.Context, id, userID string) error
}

func (s *stubSecretRepo) Create(ctx context.Context, secret *domain.Secret) error {
	if s.createFn == nil {
		return nil
	}
	return s.createFn(ctx, secret)
}

func (s *stubSecretRepo) FindByID(ctx context.Context, id string) (*domain.Secret, error) {
	if s.findByIDFn == nil {
		return nil, domain.ErrSecretNotFound
	}
	return s.findByIDFn(ctx, id)
}

func (s *stubSecretRepo) FindByUser(ctx context.Context, userID string) ([]*domain.Secret, error) {
	if s.findByUserFn == nil {
		return []*domain.Secret{}, nil
	}
	return s.findByUserFn(ctx, userID)
}

func (s *stubSecretRepo) FindByUserSince(ctx context.Context, userID string, since time.Time) ([]*domain.Secret, error) {
	if s.findByUserSinceFn == nil {
		return []*domain.Secret{}, nil
	}
	return s.findByUserSinceFn(ctx, userID, since)
}

func (s *stubSecretRepo) Update(ctx context.Context, secret *domain.Secret) error {
	if s.updateFn == nil {
		return nil
	}
	return s.updateFn(ctx, secret)
}

func (s *stubSecretRepo) SoftDelete(ctx context.Context, id, userID string) error {
	if s.softDeleteFn == nil {
		return nil
	}
	return s.softDeleteFn(ctx, id, userID)
}

// seqGenerator возвращает идентификаторы по порядку.
type seqGenerator struct {
	ids []string
	i   int
}

func (g *seqGenerator) Generate() string {
	id := g.ids[g.i]
	g.i++
	return id
}

// newTestSecretService создаёт сервис с фиксированным временем и генератором.
func newTestSecretService(repo SecretRepository) *SecretService {
	svc := NewSecretService(repo, &seqGenerator{ids: []string{"generated-id", "another-id"}})
	fixed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }
	return svc
}

// validSecret возвращает валидный секрет без ID и UserID.
func validSecret() *domain.Secret {
	return &domain.Secret{
		Type:     domain.SecretTypeCredentials,
		Name:     "Почта",
		Metadata: "личное",
		Data:     []byte("ciphertext"),
	}
}

func TestSecretServiceCreate(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		var created *domain.Secret
		repo := &stubSecretRepo{createFn: func(_ context.Context, s *domain.Secret) error {
			created = s
			return nil
		}}
		svc := newTestSecretService(repo)

		secret := validSecret()
		require.NoError(t, svc.Create(context.Background(), "user-1", secret))
		require.Same(t, secret, created)
		require.Equal(t, "generated-id", secret.ID)
		require.Equal(t, "user-1", secret.UserID)
		require.Equal(t, int64(1), secret.Version)
		require.Equal(t, svc.now(), secret.CreatedAt)
		require.Nil(t, secret.DeletedAt)
	})

	t.Run("ошибка репозитория", func(t *testing.T) {
		dbErr := errors.New("сбой")
		repo := &stubSecretRepo{createFn: func(context.Context, *domain.Secret) error { return dbErr }}
		svc := newTestSecretService(repo)

		err := svc.Create(context.Background(), "user-1", validSecret())
		require.ErrorIs(t, err, dbErr)
	})
}

func TestSecretServiceCreateValidation(t *testing.T) {
	tests := []struct {
		name   string
		modify func(s *domain.Secret)
		want   error
	}{
		{"неизвестный тип", func(s *domain.Secret) { s.Type = "unknown" }, domain.ErrInvalidSecretType},
		{"пустой тип", func(s *domain.Secret) { s.Type = "" }, domain.ErrInvalidSecretType},
		{"пустое имя", func(s *domain.Secret) { s.Name = "" }, domain.ErrInvalidSecretData},
		{"длинное имя", func(s *domain.Secret) { s.Name = string(make([]byte, maxSecretNameLen+1)) }, domain.ErrInvalidSecretData},
		{"пустые данные", func(s *domain.Secret) { s.Data = nil }, domain.ErrInvalidSecretData},
		{"слишком большие данные", func(s *domain.Secret) { s.Data = make([]byte, maxSecretSize+1) }, domain.ErrInvalidSecretData},
		{"слишком большая метаинформация", func(s *domain.Secret) { s.Metadata = string(make([]byte, maxMetadataSize+1)) }, domain.ErrInvalidSecretData},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubSecretRepo{createFn: func(context.Context, *domain.Secret) error {
				t.Fatal("репозиторий не должен вызываться при невалидных данных")
				return nil
			}}
			svc := newTestSecretService(repo)

			secret := validSecret()
			tt.modify(secret)
			err := svc.Create(context.Background(), "user-1", secret)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestSecretServiceCreateNil(t *testing.T) {
	svc := newTestSecretService(&stubSecretRepo{})
	err := svc.Create(context.Background(), "user-1", nil)
	require.ErrorIs(t, err, domain.ErrInvalidSecretData)
}

func TestSecretServiceGet(t *testing.T) {
	stored := validSecret()
	stored.ID = "secret-1"
	stored.UserID = "user-1"

	t.Run("успех", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) {
			return stored, nil
		}}
		svc := newTestSecretService(repo)

		got, err := svc.Get(context.Background(), "user-1", "secret-1")
		require.NoError(t, err)
		require.Same(t, stored, got)
	})

	t.Run("не найдено", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) {
			return nil, domain.ErrSecretNotFound
		}}
		svc := newTestSecretService(repo)

		_, err := svc.Get(context.Background(), "user-1", "absent")
		require.ErrorIs(t, err, domain.ErrSecretNotFound)
	})

	t.Run("чужой секрет", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) {
			return stored, nil
		}}
		svc := newTestSecretService(repo)

		_, err := svc.Get(context.Background(), "intruder", "secret-1")
		require.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("ошибка репозитория", func(t *testing.T) {
		dbErr := errors.New("сбой")
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) {
			return nil, dbErr
		}}
		svc := newTestSecretService(repo)

		_, err := svc.Get(context.Background(), "user-1", "secret-1")
		require.ErrorIs(t, err, dbErr)
	})
}

func TestSecretServiceList(t *testing.T) {
	t.Run("пустой", func(t *testing.T) {
		svc := newTestSecretService(&stubSecretRepo{})
		got, err := svc.List(context.Background(), "user-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("несколько", func(t *testing.T) {
		repo := &stubSecretRepo{findByUserFn: func(context.Context, string) ([]*domain.Secret, error) {
			return []*domain.Secret{validSecret(), validSecret()}, nil
		}}
		svc := newTestSecretService(repo)

		got, err := svc.List(context.Background(), "user-1")
		require.NoError(t, err)
		require.Len(t, got, 2)
	})

	t.Run("ошибка репозитория", func(t *testing.T) {
		dbErr := errors.New("сбой")
		repo := &stubSecretRepo{findByUserFn: func(context.Context, string) ([]*domain.Secret, error) {
			return nil, dbErr
		}}
		svc := newTestSecretService(repo)

		_, err := svc.List(context.Background(), "user-1")
		require.ErrorIs(t, err, dbErr)
	})
}

func TestSecretServiceUpdate(t *testing.T) {
	stored := validSecret()
	stored.ID = "secret-1"
	stored.UserID = "user-1"
	stored.Version = 3

	t.Run("успех", func(t *testing.T) {
		var updated *domain.Secret
		repo := &stubSecretRepo{
			findByIDFn: func(context.Context, string) (*domain.Secret, error) { return stored, nil },
			updateFn: func(_ context.Context, s *domain.Secret) error {
				updated = s
				return nil
			},
		}
		svc := newTestSecretService(repo)

		secret := validSecret()
		secret.ID = "secret-1"
		secret.Version = 3
		require.NoError(t, svc.Update(context.Background(), "user-1", secret))
		require.Same(t, secret, updated)
		require.Equal(t, "user-1", secret.UserID)
	})

	t.Run("чужой секрет", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) { return stored, nil }}
		svc := newTestSecretService(repo)

		secret := validSecret()
		secret.ID = "secret-1"
		secret.Version = 3
		err := svc.Update(context.Background(), "intruder", secret)
		require.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("конфликт версий", func(t *testing.T) {
		repo := &stubSecretRepo{
			findByIDFn: func(context.Context, string) (*domain.Secret, error) { return stored, nil },
			updateFn:   func(context.Context, *domain.Secret) error { return domain.ErrSecretVersionMismatch },
		}
		svc := newTestSecretService(repo)

		secret := validSecret()
		secret.ID = "secret-1"
		secret.Version = 1
		err := svc.Update(context.Background(), "user-1", secret)
		require.ErrorIs(t, err, domain.ErrSecretVersionMismatch)
	})

	t.Run("невалидные данные", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) {
			t.Fatal("репозиторий не должен вызываться при невалидных данных")
			return nil, nil
		}}
		svc := newTestSecretService(repo)

		secret := validSecret()
		secret.ID = "secret-1"
		secret.Name = ""
		err := svc.Update(context.Background(), "user-1", secret)
		require.ErrorIs(t, err, domain.ErrInvalidSecretData)
	})

	t.Run("не найдено", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) {
			return nil, domain.ErrSecretNotFound
		}}
		svc := newTestSecretService(repo)

		secret := validSecret()
		secret.ID = "absent"
		err := svc.Update(context.Background(), "user-1", secret)
		require.ErrorIs(t, err, domain.ErrSecretNotFound)
	})
}

func TestSecretServiceDelete(t *testing.T) {
	stored := validSecret()
	stored.ID = "secret-1"
	stored.UserID = "user-1"

	t.Run("успех", func(t *testing.T) {
		deleted := false
		repo := &stubSecretRepo{
			findByIDFn: func(context.Context, string) (*domain.Secret, error) { return stored, nil },
			softDeleteFn: func(_ context.Context, id, userID string) error {
				deleted = true
				require.Equal(t, "secret-1", id)
				require.Equal(t, "user-1", userID)
				return nil
			},
		}
		svc := newTestSecretService(repo)

		require.NoError(t, svc.Delete(context.Background(), "user-1", "secret-1"))
		require.True(t, deleted)
	})

	t.Run("чужой секрет", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) { return stored, nil }}
		svc := newTestSecretService(repo)

		err := svc.Delete(context.Background(), "intruder", "secret-1")
		require.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("не найдено", func(t *testing.T) {
		repo := &stubSecretRepo{findByIDFn: func(context.Context, string) (*domain.Secret, error) {
			return nil, domain.ErrSecretNotFound
		}}
		svc := newTestSecretService(repo)

		err := svc.Delete(context.Background(), "user-1", "absent")
		require.ErrorIs(t, err, domain.ErrSecretNotFound)
	})

	t.Run("ошибка удаления", func(t *testing.T) {
		dbErr := errors.New("сбой")
		repo := &stubSecretRepo{
			findByIDFn:   func(context.Context, string) (*domain.Secret, error) { return stored, nil },
			softDeleteFn: func(context.Context, string, string) error { return dbErr },
		}
		svc := newTestSecretService(repo)

		err := svc.Delete(context.Background(), "user-1", "secret-1")
		require.ErrorIs(t, err, dbErr)
	})
}

func TestSecretServiceSync(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("с since", func(t *testing.T) {
		repo := &stubSecretRepo{findByUserSinceFn: func(_ context.Context, userID string, got time.Time) ([]*domain.Secret, error) {
			require.Equal(t, "user-1", userID)
			require.Equal(t, since, got)
			return []*domain.Secret{validSecret()}, nil
		}}
		svc := newTestSecretService(repo)

		got, err := svc.Sync(context.Background(), "user-1", since)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("без since", func(t *testing.T) {
		repo := &stubSecretRepo{findByUserSinceFn: func(_ context.Context, _ string, got time.Time) ([]*domain.Secret, error) {
			require.True(t, got.IsZero())
			return []*domain.Secret{}, nil
		}}
		svc := newTestSecretService(repo)

		got, err := svc.Sync(context.Background(), "user-1", time.Time{})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("ошибка репозитория", func(t *testing.T) {
		dbErr := errors.New("сбой")
		repo := &stubSecretRepo{findByUserSinceFn: func(context.Context, string, time.Time) ([]*domain.Secret, error) {
			return nil, dbErr
		}}
		svc := newTestSecretService(repo)

		_, err := svc.Sync(context.Background(), "user-1", since)
		require.ErrorIs(t, err, dbErr)
	})
}
