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
// Тексты — в HTML-разметке MAX, всё пользовательское экранируется.
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

// slotLine — блок «что, когда, где» для подтверждений и списка записей.
func slotLine(v *model.BookingView) string {
	return fmt.Sprintf("<b>%s</b>\n🗓 %s · %s–%s\n📍 %s, %s",
		esc(v.Slot.Title), fullDate(v.Slot.StartAt), clock(v.Slot.StartAt), clock(v.Slot.EndAt),
		esc(v.Venue.Name), esc(v.Venue.Address))
}

func bookingKeyboard(v *model.BookingView, from *Origin) mc.Keyboard {
	return mc.Keyboard{
		{mc.OpenApp("QR-пропуск", "ticket_"+v.ID)},
		{mc.Callback("Перенести", "rs:"+v.ID), mc.Callback("Отменить", "cx:"+v.ID)},
		{mc.Link("Как добраться", routeURL(from, v.Venue.Lat, v.Venue.Lon)), mc.Callback("Написать студии", "msg:"+v.Venue.ID)},
	}
}

func confirmationText(v *model.BookingView, created bool) string {
	var b strings.Builder
	if created {
		b.WriteString("✅ <b>Ты записан на пробное занятие!</b>\n\n")
	} else {
		b.WriteString("👌 <b>Ты уже записан на это занятие</b>\n\n")
	}
	b.WriteString(slotLine(v))
	if trial := trialLabel(&v.Venue); trial != "" {
		b.WriteString("\n🎁 " + trial)
	}
	if v.Venue.WhatToBring != "" {
		b.WriteString("\n\n<blockquote>Что взять: " + esc(lowerFirst(v.Venue.WhatToBring)) + "</blockquote>")
	}
	fmt.Fprintf(&b, "\n🎟 Код входа: <b>%s</b>\nПокажи QR-пропуск или назови код администратору.", codeLabel(v.CheckinCode))
	if created {
		b.WriteString("\n\n<i>Напомню за сутки и за 2 часа до начала.</i>")
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
func (p *Presenter) SendConfirmation(ctx context.Context, v *model.BookingView, created bool, from *Origin) error {
	if err := p.client.SendMessage(ctx, v.MaxUserID, confirmationText(v, created), bookingKeyboard(v, from)); err != nil {
		return err
	}
	if created {
		// Точка на карте — приятное дополнение; если MAX её не примет, запись всё равно подтверждена.
		_ = p.client.SendLocation(ctx, v.MaxUserID, v.Venue.Lat, v.Venue.Lon)
	}
	return nil
}

func (p *Presenter) Reminder24h(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("⏰ <b>%s в %s — тренировка</b>\n\n%s\n📍 %s, %s\n\nПридёшь? Если планы поменялись, отмени запись — место достанется тому, кто ждёт в очереди.",
		upperFirst(dayLabel(v.Slot.StartAt, p.now())), clock(v.Slot.StartAt), esc(v.Slot.Title), esc(v.Venue.Name), esc(v.Venue.Address))
	return p.client.SendMessage(ctx, v.MaxUserID, text, mc.Keyboard{
		{mc.Callback("Иду", "go:"+v.ID), mc.Callback("Не смогу", "no:"+v.ID)},
	})
}

func (p *Presenter) Reminder2h(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("🏁 <b>Через 2 часа тренировка</b>\n\n%s в %s\n📍 %s, %s\n🎟 Код входа: <b>%s</b>",
		esc(v.Slot.Title), clock(v.Slot.StartAt), esc(v.Venue.Name), esc(v.Venue.Address), codeLabel(v.CheckinCode))
	return p.client.SendMessage(ctx, v.MaxUserID, text, mc.Keyboard{
		{mc.OpenApp("QR-пропуск", "ticket_"+v.ID), mc.Link("Как добраться", routeURL(nil, v.Venue.Lat, v.Venue.Lon))},
		{mc.Callback("Не смогу", "no:"+v.ID)},
	})
}

func (p *Presenter) AskFeedback(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("🙌 <b>Посещение отмечено</b>\n%s, %s\n\nКак прошло первое занятие? Оцени от 1 до 5.", esc(v.Slot.Title), esc(v.Venue.Name))
	row := make([]mc.Button, 0, 5)
	for i := 1; i <= 5; i++ {
		row = append(row, mc.Callback(fmt.Sprint(i), fmt.Sprintf("rt:%s:%d", v.ID, i)))
	}
	return p.client.SendMessage(ctx, v.MaxUserID, text, mc.Keyboard{row})
}

func (p *Presenter) Promoted(ctx context.Context, v *model.BookingView) error {
	text := fmt.Sprintf("🎉 <b>Освободилось место — ты записан из листа ожидания</b>\n\n%s\n\n🎟 Код входа: <b>%s</b>\n<i>Если не получается прийти — отмени запись, место уйдёт следующему.</i>",
		slotLine(v), codeLabel(v.CheckinCode))
	return p.client.SendMessage(ctx, v.MaxUserID, text, bookingKeyboard(v, nil))
}

// userLink — имя клиента ссылкой на его профиль MAX (только для числовых id).
func userLink(maxUserID, name string) string {
	if name = strings.TrimSpace(name); name == "" {
		name = "Клиент"
	}
	for _, r := range maxUserID {
		if r < '0' || r > '9' {
			return "<b>" + esc(name) + "</b>"
		}
	}
	return `<a href="max://user/` + maxUserID + `">` + esc(name) + "</a>"
}

var studioEventTitle = map[string]string{
	"booked":      "🆕 <b>Новая запись</b>",
	"cancelled":   "❌ <b>Клиент отменил запись</b>",
	"rescheduled": "🔁 <b>Клиент перенёс запись</b>",
	"going":       "👍 <b>Клиент подтвердил, что придёт</b>",
	"promoted":    "🎉 <b>Запись из листа ожидания</b>",
}

// StudioEvent — уведомление администратору студии о событии с записью.
func (p *Presenter) StudioEvent(ctx context.Context, adminMaxID, kind string, v *model.BookingView) error {
	title := studioEventTitle[kind]
	if title == "" {
		title = "<b>Изменение записи</b>"
	}
	text := fmt.Sprintf("%s\n\n👤 %s\n<b>%s</b>\n🗓 %s · %s\n🏟 %s\n<i>Занято мест: %d из %d</i>",
		title, userLink(v.MaxUserID, v.UserName), esc(v.Slot.Title), fullDate(v.Slot.StartAt), clock(v.Slot.StartAt),
		esc(v.Venue.Name), v.Slot.QuotaBooked, v.Slot.QuotaTotal)
	return p.client.SendMessage(ctx, adminMaxID, text, mc.Keyboard{{mc.OpenApp("Открыть в кабинете", "sb_"+v.ID)}})
}

// StudioMessage — администратору: клиент написал студии.
func (p *Presenter) StudioMessage(ctx context.Context, adminMaxID string, venue *model.Venue, client *model.User, text string, messageID int64) error {
	body := fmt.Sprintf("💬 <b>Новое сообщение</b> от %s\n🏟 %s\n\n<blockquote>%s</blockquote>",
		userLink(client.MaxUserID, client.Name), esc(venue.Name), esc(text))
	return p.client.SendMessage(ctx, adminMaxID, body, mc.Keyboard{{mc.OpenApp("Ответить в кабинете", fmt.Sprintf("sm_%d", messageID))}})
}

// StudioReply — клиенту: ответ студии.
func (p *Presenter) StudioReply(ctx context.Context, clientMaxID string, venue *model.Venue, text string) error {
	body := fmt.Sprintf("💬 <b>Ответ студии «%s»</b>\n\n<blockquote>%s</blockquote>", esc(venue.Name), esc(text))
	return p.client.SendMessage(ctx, clientMaxID, body, mc.Keyboard{{mc.Callback("Ответить", "msg:"+venue.ID)}})
}

// SlotCancelled — клиенту: студия отменила занятие.
func (p *Presenter) SlotCancelled(ctx context.Context, v *model.BookingView) error {
	contacts := "напиши студии здесь, в чате"
	if v.Venue.Phone != "" {
		contacts = "звони по телефону " + esc(phoneLabel(v.Venue.Phone)) + " или напиши студии здесь, в чате"
	}
	text := fmt.Sprintf("⚠️ <b>Занятие отменено студией</b>\n\n%s\n\nЗапись отменена администратором студии. Если есть вопросы — %s.",
		slotLine(v), contacts)
	return p.client.SendMessage(ctx, v.MaxUserID, text, mc.Keyboard{
		{mc.Callback("Выбрать другое время", "vs:"+v.Venue.ID)},
		{mc.Callback("Написать студии", "msg:"+v.Venue.ID)},
	})
}
