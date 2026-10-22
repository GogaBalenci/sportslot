package service

import (
	"context"
	"log"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/model"
)

// Рабочее время студий для сообщений от сервиса: ночью не будим.
const (
	workdayStart = 9
	workdayEnd   = 21
	// Через час без ответа напоминаем студии, через три — даём клиенту контакты.
	nudgeStudioAfter = time.Hour
	nudgeClientAfter = 3 * time.Hour
)

func workingHours(now time.Time) bool {
	h := now.In(catalog.Moscow).Hour()
	return h >= workdayStart && h < workdayEnd
}

// DigestStudio — строка утренней сводки по одной студии.
type DigestStudio struct {
	Venue        model.Venue
	Today        []model.BookingView
	Unread       int
	PassRequests int
	Paid         model.PaymentTotals // вчера
}

func (d DigestStudio) Empty() bool {
	return len(d.Today) == 0 && d.Unread == 0 && d.PassRequests == 0 && d.Paid.Count == 0
}

// StudioTick — утренняя сводка и напоминания о сообщениях без ответа.
func (s *StudioService) StudioTick(ctx context.Context, now time.Time) {
	if s.presenter == nil || !workingHours(now) {
		return
	}
	s.sendDigests(ctx, now)
	s.nudgeWaiting(ctx, now)
}

func (s *StudioService) sendDigests(ctx context.Context, now time.Time) {
	local := now.In(catalog.Moscow)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, catalog.Moscow)
	targets, err := s.store.DigestTargets(ctx, day)
	if err != nil {
		log.Printf("digest targets: %v", err)
		return
	}
	for _, t := range targets {
		// Сначала отметка: при сбое отправки сводка не придёт дважды.
		fresh, err := s.store.MarkDigest(ctx, t.MaxUserID, day)
		if err != nil || !fresh {
			continue
		}
		var studios []DigestStudio
		for _, id := range t.VenueIDs {
			d, err := s.digestFor(ctx, id, day)
			if err != nil {
				log.Printf("digest %s: %v", id, err)
				continue
			}
			if !d.Empty() {
				studios = append(studios, *d)
			}
		}
		if len(studios) == 0 {
			continue
		}
		if err := s.presenter.StudioDigest(ctx, t.MaxUserID, day, studios); err != nil {
			log.Printf("digest to %s: %v", t.MaxUserID, err)
		}
	}
}

func (s *StudioService) digestFor(ctx context.Context, venueID string, day time.Time) (*DigestStudio, error) {
	v, err := s.store.GetVenue(ctx, venueID)
	if err != nil {
		return nil, err
	}
	d := &DigestStudio{Venue: *v}
	views, err := s.store.VenueBookings(ctx, []string{venueID}, day, day.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	for _, b := range views {
		if b.Status == model.BookingStatusConfirmed {
			d.Today = append(d.Today, b)
		}
	}
	if d.Unread, err = s.store.UnreadMessages(ctx, []string{venueID}); err != nil {
		return nil, err
	}
	if d.PassRequests, err = s.store.RequestedPasses(ctx, []string{venueID}); err != nil {
		return nil, err
	}
	d.Paid, err = s.store.PaymentTotals(ctx, []string{venueID}, day.AddDate(0, 0, -1), day)
	return d, err
}

func (s *StudioService) nudgeWaiting(ctx context.Context, now time.Time) {
	waiting, err := s.store.WaitingThreads(ctx, now.Add(-nudgeStudioAfter), now.Add(-72*time.Hour), false)
	if err != nil {
		log.Printf("waiting threads: %v", err)
		return
	}
	for _, w := range waiting {
		venue, err := s.store.GetVenue(ctx, w.VenueID)
		if err != nil {
			continue
		}
		if !w.StudioNudged {
			if err := s.store.MarkWaitingNudged(ctx, w.LastMessageID, false); err != nil {
				continue
			}
			w := w
			s.notifyAdmins(w.VenueID, func(ctx context.Context, admin string) error {
				return s.presenter.StudioWaiting(ctx, admin, venue, w.UserName, w.Since, w.LastMessageID)
			})
		}
		if !w.ClientNotified && w.Since.Before(now.Add(-nudgeClientAfter)) && venue.Phone != "" {
			if err := s.store.MarkWaitingNudged(ctx, w.LastMessageID, true); err != nil {
				continue
			}
			if err := s.presenter.ClientWaiting(ctx, w.MaxUserID, venue); err != nil {
				log.Printf("client waiting notice: %v", err)
			}
		}
	}
}

// StatsFull — статистика кабинета: посещения, ответы на сообщения, оплаты.
type StatsFull struct {
	*model.StudioStats
	Response model.ResponseStats `json:"response"`
	Paid     model.PaymentTotals `json:"paid"`
}
