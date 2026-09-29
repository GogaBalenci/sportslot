package service

import (
	"context"
	"log"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/seed"
)

// Notifier — фоновая горутина: напоминания за сутки и за два часа, вопрос
// «Как прошло?», закрытие неявок и ежедневное обновление демо-расписания.
type Notifier struct {
	store     *postgres.Store
	presenter Presenter
	interval  time.Duration
	demo      bool
	lastSync  time.Time
}

func NewNotifier(store *postgres.Store, presenter Presenter, interval time.Duration, demo bool) *Notifier {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Notifier{store: store, presenter: presenter, interval: interval, demo: demo, lastSync: time.Now()}
}

func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()
	log.Printf("notifier: started, interval %s", n.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.Tick(ctx, time.Now())
		}
	}
}

func (n *Notifier) Tick(ctx context.Context, now time.Time) {
	n.send(ctx, "24h", now, n.store.DueReminders24h, n.presenter.Reminder24h)
	n.send(ctx, "2h", now, n.store.DueReminders2h, n.presenter.Reminder2h)

	pending, err := n.store.PendingFeedback(ctx)
	if err != nil {
		log.Printf("notifier: pending feedback: %v", err)
	}
	for i := range pending {
		if err := n.store.MarkFeedbackAsked(ctx, pending[i].ID); err != nil {
			log.Printf("notifier: mark feedback %s: %v", pending[i].ID, err)
			continue
		}
		if err := n.presenter.AskFeedback(ctx, &pending[i]); err != nil {
			log.Printf("notifier: feedback %s: %v", pending[i].ID, err)
		}
	}

	if _, err := n.store.MarkNoShows(ctx, now); err != nil {
		log.Printf("notifier: no-shows: %v", err)
	}
	if n.demo && now.Sub(n.lastSync) > 6*time.Hour {
		if _, err := seed.SyncDemo(ctx, n.store, now); err != nil {
			log.Printf("notifier: demo schedule: %v", err)
		} else {
			n.lastSync = now
		}
	}
}

func (n *Notifier) send(ctx context.Context, kind string, now time.Time,
	due func(context.Context, time.Time) ([]model.BookingView, error),
	deliver func(context.Context, *model.BookingView) error) {
	views, err := due(ctx, now)
	if err != nil {
		log.Printf("notifier: due %s reminders: %v", kind, err)
		return
	}
	for i := range views {
		// Отмечаем до отправки: лучше потерять одно напоминание, чем прислать его дважды.
		if err := n.store.MarkReminderSent(ctx, views[i].ID, kind); err != nil {
			log.Printf("notifier: mark %s reminder %s: %v", kind, views[i].ID, err)
			continue
		}
		if err := deliver(ctx, &views[i]); err != nil {
			log.Printf("notifier: send %s reminder %s: %v", kind, views[i].ID, err)
		}
	}
}
