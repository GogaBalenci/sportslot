package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	mc "sportslot/internal/maxclient"
	"sportslot/internal/model"
)

// Presenter отправляет сообщения о записях: реализует service.Presenter.
type Presenter struct {
	client *mc.Client
	now    func() time.Time
}

func NewPresenter(client *mc.Client) *Presenter {
	return &Presenter{client: client, now: time.Now}
}

func codeLabel(code string) string {
	if len(code) == 6 {
		return code[:3] + " " + code[3:]
	}
	return code
}

func slotLine(v *model.BookingView) string {
	return fmt.Sprintf("%s\n%s, %s–%s\n%s, %s", v.Slot.Title, fullDate(v.Slot.StartAt), clock(v.Slot.StartAt),
		clock(v.Slot.EndAt), v.Venue.Name, v.Venue.Address)
}

func bookingKeyboard(v *model.BookingView) mc.Keyboard {
	return mc.Keyboard{
		{mc.OpenApp("QR-пропуск", "ticket_"+v.ID)},
		{mc.Callback("Перенести", "rs:"+v.ID), mc.Callback("Отменить", "cx:"+v.ID)},
		{mc.Link("Как добраться", routeURL(v.Venue.Lat, v.Venue.Lon))},
	}
}

func confirmationText(v *model.BookingView, created bool) string {
	var b strings.Builder
	if created {
		b.WriteString("Записал тебя на пробное занятие.\n\n")
	} else {
		b.WriteString("Ты уже записан на это занятие.\n\n")
	}
	b.WriteString(slotLine(v))
	if trial := trialLabel(&v.Venue); trial != "" {
		b.WriteString("\n" + trial)
	}
	if v.Venue.WhatToBring != "" {
		b.WriteString("\n\nЧто взять: " + lowerFirst(v.Venue.WhatToBring))
	}
	fmt.Fprintf(&b, "\n\nКод входа: %s. На входе покажи QR-пропуск или назови код администратору.", codeLabel(v.CheckinCode))
	if created {
		b.WriteString("\n\nНапомню за сутки и за два часа до начала.")
	}
	return b.String()
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToLower(string(r[:1])) + string(r[1:])
}

// SendConfirmation — подтверждение записи и точка на карте отдельным сообщением.
func (p *Presenter) SendConfirmation(ctx context.Context, v *model.BookingView, created bool) error {
	if err := p.client.SendMessage(ctx, v.MaxUserID, confirmationText(v, created), bookingKeyboard(v)); err != nil {
		return err
	}
	if created {
		// Точка на карте — приятное дополнение; если MAX её не примет, запись всё равно подтверждена.
		_ = p.client.SendLocation(ctx, v.MaxUserID, v.Venue.Lat, v.Venue.Lon)
	}
	return nil
}

func (p *Presenter) Reminder24h(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("%s в %s — %s, %s.\n\nПридёшь? Если планы поменялись, отмени запись: место достанется тому, кто ждёт в очереди.",
		upperFirst(dayLabel(v.Slot.StartAt, p.now())), clock(v.Slot.StartAt), strings.ToLower(v.Slot.Title), v.Venue.Name)
	return p.client.SendMessage(ctx, v.MaxUserID, text, mc.Keyboard{
		{mc.Callback("Иду", "go:"+v.ID), mc.Callback("Не смогу", "no:"+v.ID)},
	})
}

func (p *Presenter) Reminder2h(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("Через два часа тренировка: %s в %s.\n\n%s, %s\nКод входа: %s",
		strings.ToLower(v.Slot.Title), clock(v.Slot.StartAt), v.Venue.Name, v.Venue.Address, codeLabel(v.CheckinCode))
	return p.client.SendMessage(ctx, v.MaxUserID, text, mc.Keyboard{
		{mc.OpenApp("QR-пропуск", "ticket_"+v.ID), mc.Link("Как добраться", routeURL(v.Venue.Lat, v.Venue.Lon))},
		{mc.Callback("Не смогу", "no:"+v.ID)},
	})
}

func (p *Presenter) AskFeedback(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("Отметили посещение: %s, %s. Как прошло первое занятие?", strings.ToLower(v.Slot.Title), v.Venue.Name)
	row := make([]mc.Button, 0, 5)
	for i := 1; i <= 5; i++ {
		row = append(row, mc.Callback(fmt.Sprint(i), fmt.Sprintf("rt:%s:%d", v.ID, i)))
	}
	return p.client.SendMessage(ctx, v.MaxUserID, text+"\n\nОцени от 1 до 5.", mc.Keyboard{row})
}

func (p *Presenter) Promoted(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("Освободилось место, и я записал тебя из листа ожидания.\n\n%s\n\nКод входа: %s. Если не получается прийти — отмени запись, место уйдёт следующему.",
		slotLine(v), codeLabel(v.CheckinCode))
	return p.client.SendMessage(ctx, v.MaxUserID, text, bookingKeyboard(v))
}
