package service

import (
	"context"

	"sportslot/internal/model"
)

// Presenter отправляет пользователю сообщения о его записях. Реализация —
// в пакете bot; сервисы не знают, как выглядят сообщения в MAX.
type Presenter interface {
	Reminder24h(ctx context.Context, v *model.BookingView) error
	Reminder2h(ctx context.Context, v *model.BookingView) error
	AskFeedback(ctx context.Context, v *model.BookingView) error
	Promoted(ctx context.Context, v *model.BookingView) error
}
