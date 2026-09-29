// Package bot — сценарий чат-бота СпортСлот в MAX.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"sportslot/internal/catalog"
	mc "sportslot/internal/maxclient"
	"sportslot/internal/model"
	"sportslot/internal/quiz"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/service"
)

// Incoming — событие от пользователя, уже разобранное из Update MAX.
type Incoming struct {
	MaxUserID   string
	Name        string
	Text        string
	Payload     string
	CallbackID  string
	Started     bool
	HasLocation bool
	Lat, Lon    float64
}

// state — где пользователь находится в сценарии. Хранится в dialog_states.
type state struct {
	Step      string            `json:"step"`
	Sport     string            `json:"sport,omitempty"`
	When      string            `json:"when,omitempty"`
	District  string            `json:"district,omitempty"`
	Lat       float64           `json:"lat,omitempty"`
	Lon       float64           `json:"lon,omitempty"`
	HasPoint  bool              `json:"has_point,omitempty"`
	Offset    int               `json:"offset,omitempty"`
	QuizIndex int               `json:"quiz_index,omitempty"`
	Answers   map[string]string `json:"answers,omitempty"`
}

const (
	stepMenu    = "menu"
	stepQuiz    = "quiz"
	stepWhen    = "when"
	stepWhere   = "where"
	stepResults = "results"
	pageSize    = 3
)

type Bot struct {
	client    *mc.Client
	store     *postgres.Store
	search    *service.SearchService
	bookings  *service.BookingService
	presenter *Presenter
	now       func() time.Time
}

func New(client *mc.Client, store *postgres.Store, search *service.SearchService,
	bookings *service.BookingService, presenter *Presenter) *Bot {
	return &Bot{client: client, store: store, search: search, bookings: bookings, presenter: presenter, now: time.Now}
}

// reply — короткий способ отправить сообщение текущему пользователю.
func (b *Bot) reply(ctx context.Context, in Incoming, text string, kb mc.Keyboard) {
	if err := b.client.SendMessage(ctx, in.MaxUserID, text, kb); err != nil {
		log.Printf("bot: send to %s: %v", in.MaxUserID, err)
	}
}

func (b *Bot) load(ctx context.Context, userID string) state {
	var st state
	if ok, err := b.store.LoadDialog(ctx, userID, &st); err != nil || !ok {
		st = state{Step: stepMenu}
	}
	return st
}

func (b *Bot) save(ctx context.Context, userID string, st state) {
	if err := b.store.SaveDialog(ctx, userID, st); err != nil {
		log.Printf("bot: save state %s: %v", userID, err)
	}
}

// Handle обрабатывает одно событие. Ошибки внутри сценария превращаются в
// понятные сообщения: пользователь всегда может продолжить без перезапуска.
func (b *Bot) Handle(ctx context.Context, in Incoming) {
	if in.CallbackID != "" {
		if err := b.client.AnswerCallback(ctx, in.CallbackID, ""); err != nil {
			log.Printf("bot: answer callback: %v", err)
		}
	}
	st := b.load(ctx, in.MaxUserID)

	switch {
	case in.Started:
		b.menu(ctx, in, true)
	case in.HasLocation:
		st.HasPoint, st.Lat, st.Lon, st.District = true, in.Lat, in.Lon, ""
		b.results(ctx, in, st, 0)
	case in.Payload != "":
		b.callback(ctx, in, st)
	default:
		b.text(ctx, in, st)
	}
}

func (b *Bot) callback(ctx context.Context, in Incoming, st state) {
	cmd, arg, _ := strings.Cut(in.Payload, ":")
	switch cmd {
	case "menu":
		b.menu(ctx, in, false)
	case "help":
		b.help(ctx, in)
	case "my":
		b.myBookings(ctx, in)
	case "quiz":
		st = state{Step: stepQuiz, Answers: map[string]string{}}
		b.save(ctx, in.MaxUserID, st)
		b.askQuestion(ctx, in, st)
	case "qa":
		b.quizAnswer(ctx, in, st, arg)
	case "qb":
		if st.Step == stepQuiz && st.QuizIndex > 0 {
			st.QuizIndex--
			b.save(ctx, in.MaxUserID, st)
			b.askQuestion(ctx, in, st)
			return
		}
		b.menu(ctx, in, false)
	case "sp":
		b.chooseSport(ctx, in, arg)
	case "w":
		st.When, st.Step = arg, stepWhere
		b.save(ctx, in.MaxUserID, st)
		b.askWhere(ctx, in)
	case "d":
		st.HasPoint, st.District = false, arg
		if arg == "any" {
			st.District = ""
		}
		b.results(ctx, in, st, 0)
	case "more":
		b.results(ctx, in, st, st.Offset+pageSize)
	case "res":
		b.results(ctx, in, st, st.Offset)
	case "wider":
		st.When, st.District, st.HasPoint = "any", "", false
		b.results(ctx, in, st, 0)
	case "vs":
		b.venueSlots(ctx, in, arg, st.Sport)
	case "bk":
		b.book(ctx, in, arg)
	case "wl":
		b.waitlist(ctx, in, arg)
	case "cx":
		b.confirmCancel(ctx, in, arg)
	case "cxy", "no":
		b.cancel(ctx, in, arg, cmd == "no")
	case "go":
		b.reply(ctx, in, "Отлично, ждём тебя! За два часа пришлю адрес и код входа.", nil)
		b.bookings.LogEvent(ctx, in.MaxUserID, "reminder_confirmed", map[string]interface{}{"booking_id": arg})
	case "rs":
		b.rescheduleOptions(ctx, in, arg)
	case "rst":
		bookingID, slotID, _ := strings.Cut(arg, ":")
		b.reschedule(ctx, in, bookingID, slotID)
	case "rt":
		bookingID, rating, _ := strings.Cut(arg, ":")
		b.rate(ctx, in, bookingID, rating)
	default:
		b.menu(ctx, in, false)
	}
}

func (b *Bot) text(ctx context.Context, in Incoming, st state) {
	t := strings.ToLower(strings.TrimSpace(in.Text))
	switch t {
	case "/start", "start", "начать", "меню", "/menu":
		b.menu(ctx, in, true)
		return
	case "/my", "мои записи", "мои занятия":
		b.myBookings(ctx, in)
		return
	case "/help", "помощь":
		b.help(ctx, in)
		return
	}

	switch st.Step {
	case stepWhen:
		if when := parseWhen(t); when != "" {
			st.When, st.Step = when, stepWhere
			b.save(ctx, in.MaxUserID, st)
			b.askWhere(ctx, in)
			return
		}
		b.askWhen(ctx, in, "Не разобрал, когда удобно. Выбери вариант:")
		return
	case stepWhere:
		if d, ok := catalog.ParseDistrict(t); ok {
			st.District, st.HasPoint = d.ID, false
			b.results(ctx, in, st, 0)
			return
		}
		b.reply(ctx, in, "Не нашёл такой район Ростова. Выбери из списка или отправь геолокацию.", whereKeyboard())
		return
	}

	if sport := catalog.ParseSport(t); sport != "" {
		b.chooseSport(ctx, in, sport)
		return
	}
	b.reply(ctx, in, "Я пока понимаю только кнопки и пару слов вроде «бокс» или «йога». Выбери вариант ниже.", menuKeyboard())
}

func parseWhen(t string) string {
	switch {
	case strings.Contains(t, "сегодня"):
		return "today"
	case strings.Contains(t, "завтра"):
		return "tomorrow"
	case strings.Contains(t, "выходн") || strings.Contains(t, "суббот") || strings.Contains(t, "воскрес"):
		return "weekend"
	case strings.Contains(t, "вечер") || strings.Contains(t, "будн") || strings.Contains(t, "после работы"):
		return "weekday_eve"
	case strings.Contains(t, "любое") || strings.Contains(t, "неважно") || strings.Contains(t, "не важно"):
		return "any"
	}
	return ""
}

func menuKeyboard() mc.Keyboard {
	var kb mc.Keyboard
	var row []mc.Button
	for _, s := range catalog.Sports {
		if !s.Bookable {
			continue
		}
		row = append(row, mc.Callback(s.Short, "sp:"+s.ID))
		if len(row) == 3 {
			kb = append(kb, row)
			row = nil
		}
	}
	if len(row) > 0 {
		kb = append(kb, row)
	}
	return append(kb,
		[]mc.Button{mc.Callback("Не знаю, чем заняться", "quiz")},
		[]mc.Button{mc.Callback("Мои записи", "my"), mc.OpenApp("Карта залов", "search")},
	)
}

func (b *Bot) menu(ctx context.Context, in Incoming, greet bool) {
	b.save(ctx, in.MaxUserID, state{Step: stepMenu})
	text := "Чем хочешь заняться?"
	if greet {
		name := strings.Fields(in.Name)
		hello := "Привет!"
		if len(name) > 0 {
			hello = "Привет, " + name[0] + "!"
		}
		text = hello + " Я помогу записаться на пробную тренировку рядом с домом в Ростове и не пропустить её.\n\n" +
			"Выбери вид спорта или ответь на пять вопросов — подскажу, с чего начать."
		b.bookings.LogEvent(ctx, in.MaxUserID, "start", nil)
	}
	b.reply(ctx, in, text, menuKeyboard())
}

func (b *Bot) help(ctx context.Context, in Incoming) {
	text := "Как это работает:\n\n" +
		"1. Выбираешь вид спорта, время и район — или проходишь подбор, если не знаешь, с чего начать.\n" +
		"2. Я показываю ближайшие залы. У партнёров СпортСлота можно записаться прямо здесь, по остальным даю контакты и маршрут.\n" +
		"3. После записи приходит код входа и QR-пропуск, за сутки и за два часа — напоминания.\n\n" +
		"Команды: /start — начать заново, /my — мои записи."
	b.reply(ctx, in, text, mc.Keyboard{{mc.Callback("Подобрать тренировку", "menu")}})
}

func (b *Bot) chooseSport(ctx context.Context, in Incoming, sport string) {
	if _, ok := catalog.SportByID(sport); !ok {
		b.menu(ctx, in, false)
		return
	}
	st := state{Step: stepWhen, Sport: sport}
	b.save(ctx, in.MaxUserID, st)
	b.askWhen(ctx, in, fmt.Sprintf("%s — отличный выбор. Когда удобно заниматься?", catalog.SportTitle(sport)))
}

func (b *Bot) askWhen(ctx context.Context, in Incoming, text string) {
	b.reply(ctx, in, text, mc.Keyboard{
		{mc.Callback("Сегодня", "w:today"), mc.Callback("Завтра", "w:tomorrow")},
		{mc.Callback("Будни вечером", "w:weekday_eve"), mc.Callback("Выходные", "w:weekend")},
		{mc.Callback("Любое время", "w:any")},
		{mc.Callback("Назад", "menu")},
	})
}

func whereKeyboard() mc.Keyboard {
	kb := mc.Keyboard{{mc.RequestGeo("Отправить геолокацию")}}
	for i := 0; i < len(catalog.Districts); i += 2 {
		row := []mc.Button{mc.Callback(catalog.Districts[i].Title, "d:"+catalog.Districts[i].ID)}
		if i+1 < len(catalog.Districts) {
			row = append(row, mc.Callback(catalog.Districts[i+1].Title, "d:"+catalog.Districts[i+1].ID))
		}
		kb = append(kb, row)
	}
	return append(kb, []mc.Button{mc.Callback("Не важно", "d:any")})
}

func (b *Bot) askWhere(ctx context.Context, in Incoming) {
	b.reply(ctx, in, "Где удобнее? Отправь геолокацию — покажу ближайшие залы. Или выбери район.", whereKeyboard())
}

func (b *Bot) searchQuery(st state) service.VenueSearch {
	return service.VenueSearch{Sport: st.Sport, When: st.When, District: st.District,
		Lat: st.Lat, Lon: st.Lon, HasPoint: st.HasPoint}
}

func (b *Bot) results(ctx context.Context, in Incoming, st state, offset int) {
	if st.Sport == "" {
		b.menu(ctx, in, false)
		return
	}
	if st.When == "" {
		st.When = "any"
	}
	cards, err := b.search.FindVenues(ctx, b.searchQuery(st), b.now())
	if err != nil {
		log.Printf("bot: search: %v", err)
		b.reply(ctx, in, "Не получилось выполнить поиск. Попробуй ещё раз через минуту.", mc.Keyboard{{mc.Callback("Повторить", "res")}})
		return
	}
	st.Step, st.Offset = stepResults, offset
	b.save(ctx, in.MaxUserID, st)
	if offset == 0 {
		b.bookings.LogEvent(ctx, in.MaxUserID, "search", map[string]interface{}{
			"sport": st.Sport, "when": st.When, "district": st.District, "geo": st.HasPoint, "found": len(cards)})
	}

	if len(cards) == 0 {
		b.reply(ctx, in, fmt.Sprintf("По запросу «%s, %s» ничего не нашлось.", strings.ToLower(catalog.SportTitle(st.Sport)),
			strings.ToLower(catalog.WhenTitle(st.When))), mc.Keyboard{
			{mc.Callback("Искать в любое время и по всему городу", "wider")},
			{mc.Callback("Другой вид спорта", "menu")},
		})
		return
	}
	if offset >= len(cards) {
		offset = 0
		st.Offset = 0
		b.save(ctx, in.MaxUserID, st)
	}

	if offset == 0 {
		b.reply(ctx, in, resultsHeader(st, cards), nil)
	}
	end := offset + pageSize
	if end > len(cards) {
		end = len(cards)
	}
	for _, card := range cards[offset:end] {
		text, kb := b.card(card, st)
		b.reply(ctx, in, text, kb)
	}

	footer := mc.Keyboard{}
	if end < len(cards) {
		footer = append(footer, []mc.Button{mc.Callback(fmt.Sprintf("Показать ещё (%d)", len(cards)-end), "more")})
	}
	footer = append(footer,
		[]mc.Button{mc.OpenApp("Все варианты на карте", "sport_"+st.Sport)},
		[]mc.Button{mc.Callback("Изменить поиск", "sp:"+st.Sport), mc.Callback("В начало", "menu")},
	)
	b.reply(ctx, in, "Что дальше?", footer)
}

func resultsHeader(st state, cards []service.VenueCard) string {
	where := "по всему городу"
	switch {
	case st.HasPoint:
		where = "рядом с тобой"
	case st.District != "":
		if d, ok := catalog.DistrictByID(st.District); ok {
			where = "ближе к району " + d.Title
		}
	}
	bookable := 0
	for _, c := range cards {
		if c.AvailableSlots > 0 {
			bookable++
		}
	}
	text := fmt.Sprintf("%s, %s, %s.", catalog.SportTitle(st.Sport), strings.ToLower(catalog.WhenTitle(st.When)), where)
	if bookable > 0 {
		text += fmt.Sprintf("\n\nЗаписаться прямо здесь можно в %d %s, остальные — залы из открытого каталога с контактами.",
			bookable, plural(bookable, "студию", "студии", "студий"))
	} else {
		text += "\n\nСвободных мест у партнёров на это время нет — вот залы из открытого каталога с контактами."
	}
	return text
}

func (b *Bot) card(c service.VenueCard, st state) (string, mc.Keyboard) {
	v := c.Venue
	var t strings.Builder
	t.WriteString(v.Name + "\n" + sourceLabel(&v, b.search.CatalogDate()) + "\n\n")
	place := []string{}
	if dt := districtTitle(v.District); dt != "" {
		place = append(place, dt)
	}
	place = append(place, distanceLabel(c.DistanceKM))
	t.WriteString(strings.Join(place, " · "))
	if v.Address != "" {
		t.WriteString("\n" + v.Address)
	}

	if v.BookingMode == model.BookingModeInstant {
		if trial := trialLabel(&v); trial != "" {
			t.WriteString("\n" + trial)
		}
		t.WriteString("\n\nБлижайшие занятия:")
		var row []mc.Button
		shown := 0
		for _, sl := range c.Slots {
			if shown == 3 {
				break
			}
			fmt.Fprintf(&t, "\n· %s, %s — %s, %s", dayLabel(sl.StartAt, b.now()), clock(sl.StartAt),
				strings.ToLower(sl.Title), seatsLabel(sl.QuotaAvailable))
			if sl.QuotaAvailable > 0 {
				row = append(row, mc.Callback(buttonTime(sl.StartAt), "bk:"+sl.SlotID))
			} else {
				row = append(row, mc.Callback("В очередь · "+clock(sl.StartAt), "wl:"+sl.SlotID))
			}
			shown++
		}
		kb := mc.Keyboard{}
		for _, btn := range row {
			kb = append(kb, []mc.Button{btn})
		}
		kb = append(kb, []mc.Button{mc.Callback("Другое время", "vs:"+v.ID), mc.OpenApp("Подробнее", "venue_"+v.ID)})
		return t.String(), kb
	}

	if v.OpeningHours != "" {
		t.WriteString("\nЧасы работы: " + openingHours(v.OpeningHours))
	}
	if v.Phone != "" {
		t.WriteString("\nТелефон: " + v.Phone)
	}
	t.WriteString("\n\nОнлайн-записи через СпортСлот здесь пока нет — расписание и пробное занятие уточни у зала.")
	links := []mc.Button{mc.Link("Маршрут", routeURL(v.Lat, v.Lon))}
	if v.Website != "" && strings.HasPrefix(v.Website, "http") {
		links = append(links, mc.Link("Сайт", v.Website))
	}
	return t.String(), mc.Keyboard{links, {mc.OpenApp("На карте", "venue_"+v.ID)}}
}

func (b *Bot) venueSlots(ctx context.Context, in Incoming, venueID, sport string) {
	venue, slots, err := b.search.VenueSlots(ctx, venueID, sport, b.now())
	if err != nil {
		b.reply(ctx, in, "Не нашёл расписание этого зала.", mc.Keyboard{{mc.Callback("В начало", "menu")}})
		return
	}
	if len(slots) == 0 {
		b.reply(ctx, in, venue.Name+": на ближайшие две недели занятий нет.", mc.Keyboard{{mc.Callback("В начало", "menu")}})
		return
	}
	kb := mc.Keyboard{}
	for _, sl := range slots {
		if len(kb) == 8 {
			break
		}
		label := fmt.Sprintf("%s · %s", buttonTime(sl.StartAt), shortTitle(sl.Title))
		if sl.QuotaAvailable() > 0 {
			kb = append(kb, []mc.Button{mc.Callback(label, "bk:"+sl.ID)})
		} else {
			kb = append(kb, []mc.Button{mc.Callback("Мест нет, в очередь · "+buttonTime(sl.StartAt), "wl:"+sl.ID)})
		}
	}
	kb = append(kb, []mc.Button{mc.OpenApp("Всё расписание", "venue_"+venue.ID)})
	b.reply(ctx, in, venue.Name+" — ближайшие занятия. Выбери удобное:", kb)
}

func shortTitle(title string) string {
	r := []rune(title)
	if len(r) > 22 {
		return string(r[:21]) + "…"
	}
	return title
}

func (b *Bot) book(ctx context.Context, in Incoming, slotID string) {
	view, created, err := b.bookings.Book(ctx, in.MaxUserID, in.Name, slotID, model.SourceChannelBot)
	switch {
	case err == nil:
		if err := b.presenter.SendConfirmation(ctx, view, created); err != nil {
			log.Printf("bot: confirmation: %v", err)
		}
		b.save(ctx, in.MaxUserID, state{Step: stepMenu})
	case errors.Is(err, service.ErrQuotaExceeded):
		b.reply(ctx, in, "Последнее место только что заняли. Могу поставить тебя в лист ожидания: если кто-то отменит запись, я сам запишу тебя и напишу.",
			mc.Keyboard{{mc.Callback("Встать в очередь", "wl:"+slotID)}, {mc.Callback("Выбрать другое время", "res")}})
	case errors.Is(err, service.ErrSlotStarted):
		b.reply(ctx, in, "Это занятие уже началось. Выбери другое время.", mc.Keyboard{{mc.Callback("К результатам", "res")}})
	default:
		log.Printf("bot: book %s: %v", slotID, err)
		b.reply(ctx, in, "Не получилось записаться. Попробуй ещё раз или выбери другое занятие.", mc.Keyboard{{mc.Callback("В начало", "menu")}})
	}
}

func (b *Bot) waitlist(ctx context.Context, in Incoming, slotID string) {
	pos, err := b.bookings.JoinWaitlist(ctx, in.MaxUserID, in.Name, slotID)
	if err != nil {
		b.reply(ctx, in, "Не получилось встать в очередь: занятие уже началось или его нет.", mc.Keyboard{{mc.Callback("В начало", "menu")}})
		return
	}
	b.reply(ctx, in, fmt.Sprintf("Готово, ты %d-й в листе ожидания. Как только место освободится, я запишу тебя и пришлю код входа.", pos),
		mc.Keyboard{{mc.Callback("Посмотреть другие варианты", "res")}})
}

func (b *Bot) myBookings(ctx context.Context, in Incoming) {
	_, views, err := b.bookings.MyBookings(ctx, in.MaxUserID, in.Name)
	if err != nil {
		log.Printf("bot: my bookings: %v", err)
		b.reply(ctx, in, "Не получилось загрузить записи, попробуй ещё раз.", mc.Keyboard{{mc.Callback("Повторить", "my")}})
		return
	}
	upcoming := 0
	for i := range views {
		v := &views[i]
		if v.Status != model.BookingStatusConfirmed || v.Slot.EndAt.Before(b.now()) {
			continue
		}
		upcoming++
		text := fmt.Sprintf("%s\nКод входа: %s", slotLine(v), codeLabel(v.CheckinCode))
		b.reply(ctx, in, text, bookingKeyboard(v))
	}
	if upcoming == 0 {
		b.reply(ctx, in, "Предстоящих записей нет. Подберём тренировку?", mc.Keyboard{{mc.Callback("Подобрать", "menu")}})
	}
}

func (b *Bot) confirmCancel(ctx context.Context, in Incoming, bookingID string) {
	view, err := b.bookings.Get(ctx, bookingID, in.MaxUserID)
	if err != nil || view.Status != model.BookingStatusConfirmed {
		b.reply(ctx, in, "Эта запись уже неактивна.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
		return
	}
	b.reply(ctx, in, fmt.Sprintf("Отменить запись?\n\n%s", slotLine(view)), mc.Keyboard{
		{mc.Callback("Да, отменить", "cxy:"+bookingID), mc.Callback("Оставить", "my")},
	})
}

func (b *Bot) cancel(ctx context.Context, in Incoming, bookingID string, fromReminder bool) {
	_, err := b.bookings.Cancel(ctx, bookingID, in.MaxUserID)
	switch {
	case err == nil:
		text := "Запись отменена, место освободилось для других."
		if fromReminder {
			text = "Понял, отменил запись. Спасибо, что предупредил — место достанется тому, кто ждёт."
		}
		b.reply(ctx, in, text, mc.Keyboard{{mc.Callback("Подобрать другое время", "menu")}})
	case errors.Is(err, service.ErrNotActive):
		b.reply(ctx, in, "Эта запись уже отменена или посещение отмечено.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
	default:
		log.Printf("bot: cancel %s: %v", bookingID, err)
		b.reply(ctx, in, "Не получилось отменить запись. Попробуй ещё раз.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
	}
}

func (b *Bot) rescheduleOptions(ctx context.Context, in Incoming, bookingID string) {
	view, err := b.bookings.Get(ctx, bookingID, in.MaxUserID)
	if err != nil || view.Status != model.BookingStatusConfirmed {
		b.reply(ctx, in, "Эта запись уже неактивна.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
		return
	}
	_, slots, err := b.search.VenueSlots(ctx, view.Venue.ID, view.Slot.SportType, b.now())
	if err != nil {
		b.reply(ctx, in, "Не получилось загрузить расписание.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
		return
	}
	kb := mc.Keyboard{}
	for _, sl := range slots {
		if sl.ID == view.SlotID || sl.QuotaAvailable() == 0 {
			continue
		}
		kb = append(kb, []mc.Button{mc.Callback(buttonTime(sl.StartAt)+" · "+shortTitle(sl.Title), "rst:"+bookingID+":"+sl.ID)})
		if len(kb) == 6 {
			break
		}
	}
	if len(kb) == 0 {
		b.reply(ctx, in, "Свободного времени в этом зале на ближайшие две недели нет.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
		return
	}
	kb = append(kb, []mc.Button{mc.Callback("Оставить как есть", "my")})
	b.reply(ctx, in, "На какое время перенести? Код входа останется прежним.", kb)
}

func (b *Bot) reschedule(ctx context.Context, in Incoming, bookingID, slotID string) {
	view, err := b.bookings.Reschedule(ctx, bookingID, slotID, in.MaxUserID)
	switch {
	case err == nil:
		b.reply(ctx, in, "Перенёс запись.\n\n"+slotLine(view)+"\nКод входа: "+codeLabel(view.CheckinCode), bookingKeyboard(view))
	case errors.Is(err, service.ErrQuotaExceeded):
		b.reply(ctx, in, "На это время места уже закончились.", mc.Keyboard{{mc.Callback("Выбрать другое", "rs:"+bookingID)}})
	default:
		log.Printf("bot: reschedule %s: %v", bookingID, err)
		b.reply(ctx, in, "Не получилось перенести запись.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
	}
}

func (b *Bot) rate(ctx context.Context, in Incoming, bookingID, raw string) {
	rating, _ := strconv.Atoi(raw)
	view, err := b.bookings.Rate(ctx, bookingID, in.MaxUserID, rating)
	if err != nil {
		b.reply(ctx, in, "Оценку уже не получится поставить, но спасибо!", nil)
		return
	}
	text := "Спасибо за оценку!"
	if rating <= 3 {
		text = "Спасибо, что честно. Можно попробовать другой зал или другой вид спорта — подберём?"
		b.reply(ctx, in, text, mc.Keyboard{{mc.Callback("Подобрать другое", "quiz")}, {mc.Callback("В начало", "menu")}})
		return
	}
	next, err := b.bookings.NextWeek(ctx, view)
	if err != nil {
		b.reply(ctx, in, text+" Главное теперь — не бросить. Запишись на следующее занятие, пока настрой есть.",
			mc.Keyboard{{mc.Callback("Выбрать время", "vs:"+view.Venue.ID)}})
		return
	}
	b.reply(ctx, in, fmt.Sprintf("%s Чтобы вошло в привычку, запишу на то же время через неделю — %s, %s?",
		text, dayLabel(next.StartAt, b.now()), clock(next.StartAt)), mc.Keyboard{
		{mc.Callback("Записаться", "bk:"+next.ID)},
		{mc.Callback("Выбрать другое время", "vs:"+view.Venue.ID)},
	})
}

func (b *Bot) askQuestion(ctx context.Context, in Incoming, st state) {
	q, ok := quiz.QuestionByIndex(st.QuizIndex)
	if !ok {
		b.quizResult(ctx, in, st)
		return
	}
	kb := mc.Keyboard{}
	for _, o := range q.Options {
		kb = append(kb, []mc.Button{mc.Callback(o.Title, "qa:"+q.ID+":"+o.ID)})
	}
	back := mc.Callback("Назад", "qb")
	if st.QuizIndex == 0 {
		back = mc.Callback("В начало", "menu")
	}
	kb = append(kb, []mc.Button{back})
	b.reply(ctx, in, fmt.Sprintf("Вопрос %d из %d. %s", st.QuizIndex+1, len(quiz.Questions), q.Title), kb)
}

func (b *Bot) quizAnswer(ctx context.Context, in Incoming, st state, arg string) {
	qid, oid, _ := strings.Cut(arg, ":")
	if st.Step != stepQuiz || !quiz.Valid(qid, oid) {
		st = state{Step: stepQuiz, Answers: map[string]string{}}
	}
	if st.Answers == nil {
		st.Answers = map[string]string{}
	}
	if quiz.Valid(qid, oid) {
		st.Answers[qid] = oid
		for i, q := range quiz.Questions {
			if q.ID == qid {
				st.QuizIndex = i + 1
			}
		}
	}
	b.save(ctx, in.MaxUserID, st)
	b.askQuestion(ctx, in, st)
}

func (b *Bot) quizResult(ctx context.Context, in Incoming, st state) {
	results := quiz.Recommend(st.Answers, 2)
	b.bookings.LogEvent(ctx, in.MaxUserID, "quiz_done", map[string]interface{}{"answers": st.Answers, "top": results[0].Sport})
	var t strings.Builder
	t.WriteString("Вот что тебе может подойти:\n")
	kb := mc.Keyboard{}
	for i, r := range results {
		fmt.Fprintf(&t, "\n%d. %s. %s", i+1, r.Title, r.Reason)
		kb = append(kb, []mc.Button{mc.Callback("Искать: "+strings.ToLower(r.Title), "sp:"+r.Sport)})
	}
	t.WriteString("\n\nНачни с пробного занятия — это ни к чему не обязывает.")
	kb = append(kb, []mc.Button{mc.Callback("Пройти заново", "quiz"), mc.Callback("В начало", "menu")})
	b.save(ctx, in.MaxUserID, state{Step: stepMenu})
	b.reply(ctx, in, t.String(), kb)
}
