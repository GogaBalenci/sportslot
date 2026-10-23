package service

import (
	"context"
	"errors"

	"sportslot/internal/logo"
	"sportslot/internal/model"
)

// ErrLogo — файл не подходит как логотип; текст показывается администратору.
type ErrLogo struct{ Message string }

func (e *ErrLogo) Error() string { return "logo: " + e.Message }

func (s *StudioService) editable(ctx context.Context, a Access, venueID string) (*model.Venue, error) {
	v, err := s.venue(ctx, a, venueID)
	if err != nil {
		return nil, err
	}
	if v.IsDemo() && a.Demo {
		return nil, ErrDemoReadOnly
	}
	return v, nil
}

// SetLogo проверяет картинку, приводит её к квадрату 512×512 и сохраняет.
func (s *StudioService) SetLogo(ctx context.Context, a Access, venueID string, raw []byte) (*model.Venue, error) {
	if _, err := s.editable(ctx, a, venueID); err != nil {
		return nil, err
	}
	res, err := logo.Process(raw)
	switch {
	case errors.Is(err, logo.ErrFormat):
		return nil, &ErrLogo{"Нужна картинка PNG или JPG"}
	case errors.Is(err, logo.ErrTooSmall):
		return nil, &ErrLogo{"Слишком маленькая картинка: нужно не меньше 256×256 пикселей"}
	case errors.Is(err, logo.ErrTooLarge):
		return nil, &ErrLogo{"Слишком большой файл: до 5 МБ и не больше 6000 пикселей по стороне"}
	case err != nil:
		return nil, err
	}
	if err := s.store.SaveLogo(ctx, venueID, res.ContentType, res.Version, res.Data); err != nil {
		return nil, err
	}
	s.event(ctx, a, "studio_logo", map[string]interface{}{"venue_id": venueID})
	return s.store.GetVenue(ctx, venueID)
}

func (s *StudioService) DeleteLogo(ctx context.Context, a Access, venueID string) error {
	if _, err := s.editable(ctx, a, venueID); err != nil {
		return err
	}
	return s.store.DeleteLogo(ctx, venueID)
}
