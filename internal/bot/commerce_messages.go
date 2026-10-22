package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	mc "sportslot/internal/maxclient"
	"sportslot/internal/model"
	"sportslot/internal/service"
)

func money(rub int) string {
	s := fmt.Sprint(rub)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String() + " ₽"
}

func passLine(up *model.UserPass) string {
	return fmt.Sprintf("<b>%s</b> · %d %s · %s", esc(up.Title), up.VisitsTotal,
		plural(up.VisitsTotal, "занятие", "занятия", "занятий"), money(up.Price))
}

// --- Клиенту ---

// FriendInvite — сообщение со ссылкой для друга: его удобно переслать.
func (p *Presenter) FriendInvite(ctx context.Context, hostMaxID string, guest *model.BookingView, link string) error {
	if err := p.client.SendMessage(ctx, hostMaxID,
		"👥 <b>Место для друга забронировано</b>\nПерешли другу сообщение ниже: по ссылке он получит свой QR-пропуск и напоминания. Пока друг не открыл ссылку, второй пропуск — в твоих записях.", nil); err != nil {
		return err
	}
	text := fmt.Sprintf("🏃 <b>Пойдём вместе на тренировку?</b>\n\n%s\n\nЯ записал нас обоих. Открой ссылку, чтобы получить свой пропуск в СпортСлот:\n%s",
		slotLine(guest), link)
	return p.client.SendMessage(ctx, hostMaxID, text, nil)
}

func (p *Presenter) FriendJoined(ctx context.Context, hostMaxID, friendName string, v *model.BookingView) error {
	name := strings.Fields(friendName)
	who := "Друг"
	if len(name) > 0 {
		who = esc(name[0])
	}
	text := fmt.Sprintf("🤝 <b>%s принял приглашение</b>\n%s, %s в %s.\nУ каждого теперь свой пропуск и свои напоминания.",
		who, esc(v.Slot.Title), dayLabel(v.Slot.StartAt, p.now()), clock(v.Slot.StartAt))
	return p.client.SendMessage(ctx, hostMaxID, text, nil)
}

func (p *Presenter) PaymentReceived(ctx context.Context, pay *model.Payment, v *model.BookingView, up *model.UserPass) error {
	var text string
	var kb mc.Keyboard
	switch {
	case v != nil:
		text = fmt.Sprintf("💳 <b>Оплата получена — %s</b>\n\n%s\n\nНа месте платить не нужно, просто покажи QR-пропуск.",
			money(pay.Amount/100), slotLine(v))
		kb = mc.Keyboard{{mc.OpenApp("QR-пропуск", "ticket_"+v.ID)}}
	case up != nil:
		text = fmt.Sprintf("💳 <b>Абонемент оплачен и активен</b>\n\n%s\n🏟 %s\nДействует до %s. Визиты списываются при записи, при отмене возвращаются.",
			passLine(up), esc(up.VenueName), validUntil(up))
		kb = mc.Keyboard{{mc.Callback("Выбрать занятие", "vs:"+up.VenueID)}}
	default:
		text = fmt.Sprintf("💳 <b>Оплата получена — %s</b>", money(pay.Amount/100))
	}
	return p.client.SendMessage(ctx, pay.MaxUserID, text, kb)
}

func validUntil(up *model.UserPass) string {
	if up.ValidUntil == nil {
		return "—"
	}
	return fullDate(*up.ValidUntil)
}

func (p *Presenter) PassActivated(ctx context.Context, up *model.UserPass) error {
	text := fmt.Sprintf("🎟 <b>Абонемент активен</b>\n\n%s\n🏟 %s\nДействует до %s. Записывайся как обычно — визит спишется автоматически.",
		passLine(up), esc(up.VenueName), validUntil(up))
	return p.client.SendMessage(ctx, up.MaxUserID, text, mc.Keyboard{{mc.Callback("Выбрать занятие", "vs:"+up.VenueID)}})
}

func (p *Presenter) PassDeclined(ctx context.Context, up *model.UserPass) error {
	text := fmt.Sprintf("Студия «%s» отклонила заявку на абонемент «%s». Если это ошибка — напиши студии.",
		esc(up.VenueName), esc(up.Title))
	return p.client.SendMessage(ctx, up.MaxUserID, text, mc.Keyboard{{mc.Callback("Написать студии", "msg:"+up.VenueID)}})
}

// ClientWaiting — клиенту: студия долго не отвечает, вот как связаться напрямую.
func (p *Presenter) ClientWaiting(ctx context.Context, clientMaxID string, venue *model.Venue) error {
	text := fmt.Sprintf("⏳ Студия «%s» пока не ответила на твоё сообщение. Если вопрос срочный, позвони: <b>%s</b>.",
		esc(venue.Name), esc(phoneLabel(venue.Phone)))
	return p.client.SendMessage(ctx, clientMaxID, text, nil)
}

// --- Администратору студии ---

func (p *Presenter) StudioPassRequest(ctx context.Context, adminMaxID string, up *model.UserPass) error {
	text := fmt.Sprintf("🎟 <b>Заявка на абонемент</b>\n\n👤 %s\n%s\n🏟 %s\n\nКлиент оплатит в студии. Когда получите оплату, подтвердите заявку в кабинете — абонемент включится.",
		userLink(up.MaxUserID, up.UserName), passLine(up), esc(up.VenueName))
	return p.client.SendMessage(ctx, adminMaxID, text, mc.Keyboard{{mc.OpenApp("Открыть заявки", "studio_passes")}})
}

func (p *Presenter) StudioPayment(ctx context.Context, adminMaxID string, pay *model.Payment, venue *model.Venue) error {
	what := "пробное занятие"
	if pay.Kind == model.PaymentKindPass {
		what = "абонемент"
	}
	text := fmt.Sprintf("💳 <b>Оплата онлайн: %s</b>\n\n👤 %s оплатил %s\n<i>%s</i>\n🏟 %s",
		money(pay.Amount/100), userLink(pay.MaxUserID, pay.UserName), what, esc(pay.Description), esc(venue.Name))
	return p.client.SendMessage(ctx, adminMaxID, text, nil)
}

func (p *Presenter) StudioWaiting(ctx context.Context, adminMaxID string, venue *model.Venue, clientName string, since time.Time, messageID int64) error {
	if strings.TrimSpace(clientName) == "" {
		clientName = "Клиент"
	}
	text := fmt.Sprintf("⏳ <b>%s ждёт ответа</b> с %s\n🏟 %s\n\nОтветить быстро — лучший способ, чтобы клиент дошёл до занятия.",
		esc(clientName), clock(since), esc(venue.Name))
	return p.client.SendMessage(ctx, adminMaxID, text, mc.Keyboard{{mc.OpenApp("Ответить в кабинете", fmt.Sprintf("sm_%d", messageID))}})
}

// StudioDigest — утренняя сводка: записи на сегодня, сообщения, заявки, оплаты.
func (p *Presenter) StudioDigest(ctx context.Context, adminMaxID string, day time.Time, studios []service.DigestStudio) error {
	var b strings.Builder
	fmt.Fprintf(&b, "☀️ <b>Доброе утро! Сводка на %s</b>", fullDate(day))
	for _, d := range studios {
		fmt.Fprintf(&b, "\n\n🏟 <b>%s</b>", esc(d.Venue.Name))
		if len(d.Today) == 0 {
			b.WriteString("\nСегодня записей нет")
		} else {
			fmt.Fprintf(&b, "\nСегодня %d %s:", len(d.Today), plural(len(d.Today), "запись", "записи", "записей"))
			for i, v := range d.Today {
				if i == 5 {
					fmt.Fprintf(&b, "\n…и ещё %d", len(d.Today)-5)
					break
				}
				name := strings.TrimSpace(v.UserName)
				if name == "" {
					name = "Клиент"
				}
				if v.IsGuest {
					name += " (друг)"
				}
				fmt.Fprintf(&b, "\n• %s — %s, %s", clock(v.Slot.StartAt), esc(name), esc(v.Slot.Title))
			}
		}
		if d.Unread > 0 {
			fmt.Fprintf(&b, "\n💬 %d %s без ответа", d.Unread, plural(d.Unread, "новое сообщение", "новых сообщения", "новых сообщений"))
		}
		if d.PassRequests > 0 {
			fmt.Fprintf(&b, "\n🎟 %d %s на абонемент", d.PassRequests, plural(d.PassRequests, "заявка", "заявки", "заявок"))
		}
		if d.Paid.Count > 0 {
			fmt.Fprintf(&b, "\n💳 Вчера оплат онлайн: %d на %s", d.Paid.Count, money(d.Paid.Amount))
		}
	}
	return p.client.SendMessage(ctx, adminMaxID, b.String(), mc.Keyboard{{mc.OpenApp("Открыть кабинет", "studio")}})
}
