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
	"sportslot/internal/geocode"
	mc "sportslot/internal/maxclient"
	"sportslot/internal/model"
	"sportslot/internal/quiz"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/routing"
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
	Step      string  `json:"step"`
	Sport     string  `json:"sport,omitempty"`
	When      string  `json:"when,omitempty"`
	District  string  `json:"district,omitempty"`
	Lat       float64 `json:"lat,omitempty"`
	Lon       float64 `json:"lon,omitempty"`
	HasPoint  bool    `json:"has_point,omitempty"`
	Address   string  `json:"address,omitempty"`
	Mode      string  `json:"mode,omitempty"`
	Limit     int     `json:"limit,omitempty"`
	Offset    int     `json:"offset,omitempty"`
	QuizIndex int     `json:"quiz_index,omitempty"`
	// MsgVenue — студия, которой пользователь пишет сообщение.
	MsgVenue string            `json:"msg_venue,omitempty"`
	Answers  map[string]string `json:"answers,omitempty"`
}

const (
	stepMenu    = "menu"
	stepQuiz    = "quiz"
	stepWhen    = "when"
	stepWhere   = "where"
	stepTravel  = "travel"
	stepMessage = "message"
	stepResults = "results"
	pageSize    = 3
)

type Bot struct {
	client    *mc.Client
	store     *postgres.Store
	search    *service.SearchService
	bookings  *service.BookingService
	presenter *Presenter
	geocoder  *geocode.Geocoder
	studio    *service.StudioService
	now       func() time.Time
}

// SetStudio включает переписку клиентов со студиями.
func (b *Bot) SetStudio(s *service.StudioService) { b.studio = s }

// SetGeocoder включает поиск по адресу, который пользователь пишет текстом.
func (b *Bot) SetGeocoder(g *geocode.Geocoder) { b.geocoder = g }

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
		st.Address = ""
		st.Mode, st.Limit = "", 0
		if err := b.store.SaveLocation(ctx, in.MaxUserID, in.Lat, in.Lon, ""); err != nil {
			log.Printf("bot: save location: %v", err)
		}
		if st.Sport == "" {
			b.save(ctx, in.MaxUserID, st)
			b.reply(ctx, in, "Запомнил, где ты. Выбери вид спорта — покажу залы рядом.", menuKeyboard())
			return
		}
		st.Step = stepTravel
		b.save(ctx, in.MaxUserID, st)
		b.askTravel(ctx, in)
	case in.Payload != "":
		b.callback(ctx, in, st)
	default:
		b.text(ctx, in, st)
	}
}

func (b *Bot) callback(ctx context.Context, in Incoming, st state) {
	cmd, arg, _ := strings.Cut(in.Payload, ":")
	if st.Step == stepMessage && cmd != "msg" {
		st.Step, st.MsgVenue = stepMenu, ""
		b.save(ctx, in.MaxUserID, st)
	}
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
	case "tm":
		mode, limit, _ := strings.Cut(arg, ":")
		st.Mode, st.Limit = "", 0
		if m, ok := routing.ParseMode(mode); ok && st.HasPoint {
			st.Mode = string(m)
			st.Limit, _ = strconv.Atoi(limit)
		}
		b.results(ctx, in, st, 0)
	case "d":
		st.HasPoint, st.Mode, st.Limit, st.District = false, "", 0, arg
		if arg == "any" {
			st.District = ""
		}
		b.results(ctx, in, st, 0)
	case "more":
		b.results(ctx, in, st, st.Offset+pageSize)
	case "res":
		b.results(ctx, in, st, st.Offset)
	case "wider":
		st.When, st.District, st.HasPoint, st.Mode, st.Limit = "any", "", false, "", 0
		b.results(ctx, in, st, 0)
	case "vs":
		b.venueSlots(ctx, in, arg, st.Sport)
	case "bk":
		b.book(ctx, in, st, arg)
	case "wl":
		b.waitlist(ctx, in, arg)
	case "cx":
		b.confirmCancel(ctx, in, arg)
	case "cxy", "no":
		b.cancel(ctx, in, arg, cmd == "no")
	case "go":
		if err := b.bookings.MarkGoing(ctx, arg, in.MaxUserID); err != nil {
			b.reply(ctx, in, "Эта запись уже неактивна.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
			return
		}
		b.reply(ctx, in, "👍 <b>Отлично, ждём тебя!</b>\nЗа два часа пришлю адрес и код входа. Студия видит, что ты придёшь.", nil)
	case "msg":
		b.askMessage(ctx, in, st, arg)
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
	case "/id":
		b.reply(ctx, in, "Твой ID в MAX: <code>"+esc(in.MaxUserID)+"</code>\n<i>Он нужен, чтобы назначить владельца платформы (PLATFORM_ADMIN_IDS).</i>", nil)
		return
	}
	if st.Step == stepMessage && st.MsgVenue != "" {
		b.sendMessage(ctx, in, st)
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
	case stepTravel:
		if mode, limit := parseTravel(t); mode != "" {
			st.Mode, st.Limit = string(mode), limit
			b.results(ctx, in, st, 0)
			return
		}
		b.askTravel(ctx, in)
		return
	case stepWhere:
		if d, ok := catalog.ParseDistrict(t); ok {
			st.District, st.HasPoint = d.ID, false
			b.results(ctx, in, st, 0)
			return
		}
		b.addressSearch(ctx, in, st, in.Text)
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
	text := "<b>Чем хочешь заняться?</b>\nВыбери вид спорта или пройди подбор."
	if greet {
		name := strings.Fields(in.Name)
		hello := "Привет!"
		if len(name) > 0 {
			hello = "Привет, " + esc(name[0]) + "!"
		}
		text = "👋 <b>" + hello + "</b>\n\nПомогу записаться на пробную тренировку рядом с домом в Ростове и не пропустить её.\n\n" +
			"🏋️ Выбери вид спорта\n✨ или ответь на 5 вопросов — подскажу, с чего начать"
		b.bookings.LogEvent(ctx, in.MaxUserID, "start", nil)
	}
	b.reply(ctx, in, text, menuKeyboard())
}

func (b *Bot) help(ctx context.Context, in Incoming) {
	text := "<b>Как это работает</b>\n\n" +
		"1️⃣ Выбираешь вид спорта и время — или проходишь подбор, если не знаешь, с чего начать.\n" +
		"2️⃣ Отправляешь геолокацию или адрес — считаю время в пути до каждого зала.\n" +
		"3️⃣ У партнёров СпортСлота записываешься прямо здесь, по остальным залам даю контакты и маршрут.\n" +
		"4️⃣ Получаешь код входа и QR-пропуск, за сутки и за 2 часа — напоминания.\n\n" +
		"<i>Команды: /start — начать заново, /my — мои записи.</i>"
	b.reply(ctx, in, text, mc.Keyboard{{mc.Callback("Подобрать тренировку", "menu")}})
}

func (b *Bot) chooseSport(ctx context.Context, in Incoming, sport string) {
	if _, ok := catalog.SportByID(sport); !ok {
		b.menu(ctx, in, false)
		return
	}
	st := state{Step: stepWhen, Sport: sport}
	b.save(ctx, in.MaxUserID, st)
	b.askWhen(ctx, in, fmt.Sprintf("<b>%s</b> — отличный выбор!\n\n🗓 Когда удобно заниматься?", catalog.SportTitle(sport)))
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
	// Кнопка геолокации в чате работает в мобильном MAX; на компьютере и в
	// веб-версии точку определяет мини-приложение.
	kb := mc.Keyboard{{mc.RequestGeo("Отправить геолокацию")}, {mc.OpenApp("Определить через приложение", "geo")}}
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
	b.reply(ctx, in, "📍 <b>Откуда будешь добираться?</b>\n\nОтправь геолокацию — посчитаю время в пути до каждого зала. Или выбери район.\n\n"+
		"<i>На компьютере нажми «Определить через приложение» или просто напиши адрес, например «Большая Садовая, 1».</i>", whereKeyboard())
}

func (b *Bot) searchQuery(st state) service.VenueSearch {
	return service.VenueSearch{Sport: st.Sport, When: st.When, District: st.District,
		Lat: st.Lat, Lon: st.Lon, HasPoint: st.HasPoint, Mode: routing.Mode(st.Mode), MaxMinutes: st.Limit}
}

func travelKeyboard() mc.Keyboard {
	return mc.Keyboard{
		{mc.Callback("Пешком · до 15 мин", "tm:walking:15")},
		{mc.Callback("Пешком · до 30 мин", "tm:walking:30")},
		{mc.Callback("На транспорте · до 20 мин", "tm:transit:20")},
		{mc.Callback("На транспорте · до 40 мин", "tm:transit:40")},
		{mc.Callback("На машине · до 20 мин", "tm:driving:20")},
		{mc.Callback("Время в пути не важно", "tm:any:0")},
	}
}

func (b *Bot) askTravel(ctx context.Context, in Incoming) {
	b.reply(ctx, in, "🧭 <b>Как будешь добираться?</b>\n\nВыбери способ и сколько готов тратить на дорогу — посчитаю время по реальным маршрутам с учётом общественного транспорта.", travelKeyboard())
}

// parseTravel понимает ответы вроде «пешком 15» или «на автобусе минут 30».
func parseTravel(t string) (routing.Mode, int) {
	var mode routing.Mode
	switch {
	case strings.Contains(t, "пешк") || strings.Contains(t, "ногами"):
		mode = routing.Walking
	case strings.Contains(t, "транспорт") || strings.Contains(t, "автобус") || strings.Contains(t, "трамва") || strings.Contains(t, "маршрутк"):
		mode = routing.Transit
	case strings.Contains(t, "машин") || strings.Contains(t, "авто") || strings.Contains(t, "такси"):
		mode = routing.Driving
	default:
		return "", 0
	}
	limit := 0
	for _, f := range strings.FieldsFunc(t, func(r rune) bool { return r < '0' || r > '9' }) {
		if n, err := strconv.Atoi(f); err == nil && n > 0 && n <= 120 {
			limit = n
			break
		}
	}
	if limit == 0 {
		limit = map[routing.Mode]int{routing.Walking: 15, routing.Transit: 20, routing.Driving: 20}[mode]
	}
	return mode, limit
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
			"sport": st.Sport, "when": st.When, "district": st.District, "geo": st.HasPoint,
			"mode": st.Mode, "limit": st.Limit, "found": len(cards)})
	}

	if len(cards) == 0 {
		b.reply(ctx, in, fmt.Sprintf("🔍 <b>Ничего не нашлось</b>\nПо запросу «%s, %s» сейчас нет вариантов.", strings.ToLower(catalog.SportTitle(st.Sport)),
			strings.ToLower(catalog.WhenTitle(st.When))), emptyKeyboard(st))
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

func emptyKeyboard(st state) mc.Keyboard {
	kb := mc.Keyboard{}
	if st.HasPoint && st.Mode != "" && st.Limit > 0 && st.Limit < 60 {
		more := st.Limit * 2
		if more > 60 {
			more = 60
		}
		mode := routing.Mode(st.Mode)
		kb = append(kb, []mc.Button{mc.Callback(fmt.Sprintf("Искать до %d мин %s", more, mode.Title()), fmt.Sprintf("tm:%s:%d", mode, more))})
	}
	return append(kb,
		[]mc.Button{mc.Callback("Искать в любое время и по всему городу", "wider")},
		[]mc.Button{mc.Callback("Другой вид спорта", "menu")},
	)
}

func resultsHeader(st state, cards []service.VenueCard) string {
	where := "по всему городу"
	switch {
	case st.HasPoint && st.Mode != "" && st.Limit > 0:
		where = fmt.Sprintf("рядом с тобой — до %d мин %s", st.Limit, routing.Mode(st.Mode).Title())
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
	beyond := 0
	for _, c := range cards {
		if c.BeyondLimit {
			beyond++
		}
	}
	text := fmt.Sprintf("🔍 <b>%s · %s</b>\n%s", catalog.SportTitle(st.Sport), strings.ToLower(catalog.WhenTitle(st.When)), upperFirst(where))
	if st.Address != "" {
		text += "\n📍 от адреса " + esc(st.Address)
	}
	if bookable > 0 {
		text += fmt.Sprintf("\n\n⚡ Записаться прямо здесь можно в <b>%d %s</b>, в остальных залах — напрямую, дам контакты и маршрут.",
			bookable, plural(bookable, "студию", "студии", "студий"))
	} else {
		text += "\n\nСвободных мест с онлайн-записью на это время нет — вот залы, где можно записаться напрямую."
	}
	if beyond > 0 && beyond < len(cards) {
		text += fmt.Sprintf("\n\n<i>В пределах %d мин нашлось немного — в конце добавил залы чуть дальше.</i>", st.Limit)
	} else if beyond > 0 {
		text += fmt.Sprintf("\n\n<i>В пределах %d мин ничего нет — показываю ближайшие залы чуть дальше.</i>", st.Limit)
	}
	return text
}

func (b *Bot) card(c service.VenueCard, st state) (string, mc.Keyboard) {
	v := c.Venue
	from := originOf(st)
	var t strings.Builder
	t.WriteString("<b>" + esc(v.Name) + "</b>\n")
	if v.BookingMode == model.BookingModeInstant {
		t.WriteString("⚡ <i>Онлайн-запись через СпортСлот</i>\n")
	} else {
		t.WriteString("<i>Запись напрямую в зале</i>\n")
	}
	if c.BeyondLimit {
		t.WriteString("<i>Чуть дальше выбранного времени</i>\n")
	}
	t.WriteString("\n")

	place := []string{}
	if dt := districtTitle(v.District); dt != "" {
		place = append(place, dt)
	}
	if v.Address != "" {
		place = append(place, esc(v.Address))
	}
	if len(place) > 0 {
		t.WriteString("📍 " + strings.Join(place, " · ") + "\n")
	}
	if c.Travel != nil {
		t.WriteString(modeIcon(c.Travel.Mode) + " <b>" + travelLabel(c.Travel) + "</b>\n")
	} else {
		t.WriteString("📏 " + distanceLabel(c.DistanceKM) + "\n")
	}

	if v.BookingMode == model.BookingModeInstant {
		if trial := trialLabel(&v); trial != "" {
			t.WriteString("🎁 " + trial + "\n")
		}
		t.WriteString("\n<b>Ближайшие занятия</b>")
		var row []mc.Button
		shown := 0
		for _, sl := range c.Slots {
			if shown == 3 {
				break
			}
			fmt.Fprintf(&t, "\n▫️ %s, %s — %s · <i>%s</i>", upperFirst(dayLabel(sl.StartAt, b.now())), clock(sl.StartAt),
				esc(lowerFirst(sl.Title)), seatsLabel(sl.QuotaAvailable))
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
		kb = append(kb,
			[]mc.Button{mc.Callback("Другое время", "vs:"+v.ID), mc.OpenApp("Подробнее", "venue_"+v.ID)},
			[]mc.Button{mc.Link("Маршрут", routeURL(from, v.Lat, v.Lon))},
		)
		return t.String(), kb
	}

	if v.OpeningHours != "" {
		t.WriteString("🕒 " + esc(openingHours(v.OpeningHours)) + "\n")
	}
	if v.Phone != "" {
		t.WriteString("📞 " + esc(phoneLabel(v.Phone)) + "\n")
	}
	t.WriteString("\n<i>Расписание и пробное занятие уточни у администратора зала.</i>")
	links := []mc.Button{mc.Link("Маршрут", routeURL(from, v.Lat, v.Lon))}
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
		text, kb := "<b>"+esc(venue.Name)+"</b>\nНа ближайшие две недели занятий нет.", mc.Keyboard{{mc.Callback("В начало", "menu")}}
		if venue.BookingMode == model.BookingModeInstant {
			text += " Можно спросить студию, когда появятся."
			kb = append(mc.Keyboard{{mc.Callback("Написать студии", "msg:"+venue.ID)}}, kb...)
		}
		b.reply(ctx, in, text, kb)
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
	last := []mc.Button{mc.OpenApp("Всё расписание", "venue_"+venue.ID)}
	if venue.BookingMode == model.BookingModeInstant {
		last = append(last, mc.Callback("Написать студии", "msg:"+venue.ID))
	}
	kb = append(kb, last)
	b.reply(ctx, in, "🗓 <b>"+esc(venue.Name)+"</b>\nБлижайшие занятия — выбери удобное:", kb)
}

func shortTitle(title string) string {
	r := []rune(title)
	if len(r) > 22 {
		return string(r[:21]) + "…"
	}
	return title
}

func (b *Bot) book(ctx context.Context, in Incoming, st state, slotID string) {
	view, created, err := b.bookings.Book(ctx, in.MaxUserID, in.Name, slotID, model.SourceChannelBot)
	switch {
	case err == nil:
		if err := b.presenter.SendConfirmation(ctx, view, created, originOf(st)); err != nil {
			log.Printf("bot: confirmation: %v", err)
		}
		b.save(ctx, in.MaxUserID, state{Step: stepMenu})
	case errors.Is(err, service.ErrQuotaExceeded):
		b.reply(ctx, in, "😕 <b>Последнее место только что заняли</b>\n\n Могу поставить тебя в лист ожидания: если кто-то отменит запись, я сам запишу тебя и напишу.",
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
	b.reply(ctx, in, fmt.Sprintf("⏳ <b>Ты %d-й в листе ожидания</b>\n\nКак только место освободится, я сам запишу тебя и пришлю код входа.", pos),
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
		text := fmt.Sprintf("%s\n🎟 Код входа: <b>%s</b>", slotLine(v), codeLabel(v.CheckinCode))
		b.reply(ctx, in, text, bookingKeyboard(v, nil))
	}
	if upcoming == 0 {
		b.reply(ctx, in, "📭 <b>Предстоящих записей нет</b>\nПодберём тренировку?", mc.Keyboard{{mc.Callback("Подобрать", "menu")}})
	}
}

func (b *Bot) confirmCancel(ctx context.Context, in Incoming, bookingID string) {
	view, err := b.bookings.Get(ctx, bookingID, in.MaxUserID)
	if err != nil || view.Status != model.BookingStatusConfirmed {
		b.reply(ctx, in, "Эта запись уже неактивна.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
		return
	}
	b.reply(ctx, in, fmt.Sprintf("❓ <b>Отменить запись?</b>\n\n%s", slotLine(view)), mc.Keyboard{
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
	b.reply(ctx, in, "🔁 <b>На какое время перенести?</b>\nКод входа останется прежним.", kb)
}

func (b *Bot) reschedule(ctx context.Context, in Incoming, bookingID, slotID string) {
	view, err := b.bookings.Reschedule(ctx, bookingID, slotID, in.MaxUserID)
	switch {
	case err == nil:
		b.reply(ctx, in, "🔁 <b>Запись перенесена</b>\n\n"+slotLine(view)+"\n🎟 Код входа: <b>"+codeLabel(view.CheckinCode)+"</b>", bookingKeyboard(view, nil))
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
	text := "💪 <b>Спасибо за оценку!</b>"
	if rating <= 3 {
		text = "<b>Спасибо, что честно.</b>\nМожно попробовать другой зал или другой вид спорта — подберём?"
		b.reply(ctx, in, text, mc.Keyboard{{mc.Callback("Подобрать другое", "quiz")}, {mc.Callback("В начало", "menu")}})
		return
	}
	next, err := b.bookings.NextWeek(ctx, view)
	if err != nil {
		b.reply(ctx, in, text+"\nГлавное теперь — не бросить. Запишись на следующее занятие, пока настрой есть.",
			mc.Keyboard{{mc.Callback("Выбрать время", "vs:"+view.Venue.ID)}})
		return
	}
	b.reply(ctx, in, fmt.Sprintf("%s\nЧтобы вошло в привычку, запишу на то же время через неделю — <b>%s, %s</b>?",
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
	b.reply(ctx, in, fmt.Sprintf("✨ <b>Вопрос %d из %d</b>\n\n%s", st.QuizIndex+1, len(quiz.Questions), q.Title), kb)
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
	t.WriteString("🎯 <b>Вот что тебе может подойти</b>\n")
	kb := mc.Keyboard{}
	medals := []string{"🥇", "🥈", "🥉"}
	for i, r := range results {
		fmt.Fprintf(&t, "\n%s <b>%s</b>\n<i>%s</i>\n", medals[i%len(medals)], r.Title, esc(r.Reason))
		kb = append(kb, []mc.Button{mc.Callback("Искать: "+strings.ToLower(r.Title), "sp:"+r.Sport)})
	}
	t.WriteString("\nНачни с пробного занятия — это ни к чему не обязывает.")
	kb = append(kb, []mc.Button{mc.Callback("Пройти заново", "quiz"), mc.Callback("В начало", "menu")})
	b.save(ctx, in.MaxUserID, state{Step: stepMenu})
	b.reply(ctx, in, t.String(), kb)
}

// addressSearch ищет адрес, написанный текстом, — это замена кнопке геолокации,
// которая работает не во всех клиентах MAX (например, в веб-версии).
func (b *Bot) addressSearch(ctx context.Context, in Incoming, st state, text string) {
	if !b.geocoder.Enabled() {
		b.reply(ctx, in, "Не нашёл такой район Ростова. Выбери из списка, отправь геолокацию или напиши адрес.", whereKeyboard())
		return
	}
	place, err := b.geocoder.Find(ctx, text)
	if err != nil {
		msg := "Не нашёл такой адрес в Ростове. Попробуй написать улицу и дом, например «Большая Садовая, 1», или выбери район."
		if !errors.Is(err, geocode.ErrNotFound) {
			log.Printf("bot: geocode: %v", err)
			msg = "Поиск по адресу сейчас недоступен. Выбери район или отправь геолокацию с телефона."
		}
		b.reply(ctx, in, msg, whereKeyboard())
		return
	}
	st.HasPoint, st.Lat, st.Lon, st.District = true, place.Lat, place.Lon, ""
	st.Address = place.Address
	st.Mode, st.Limit, st.Step = "", 0, stepTravel
	if err := b.store.SaveLocation(ctx, in.MaxUserID, place.Lat, place.Lon, place.Address); err != nil {
		log.Printf("bot: save location: %v", err)
	}
	b.save(ctx, in.MaxUserID, st)
	b.bookings.LogEvent(ctx, in.MaxUserID, "address", nil)
	b.reply(ctx, in, "📍 Считаю от адреса: <b>"+esc(place.Address)+"</b>", nil)
	b.askTravel(ctx, in)
}

// LocationFromApp — геолокация пришла из мини-приложения. Если пользователь
// был на шаге выбора места, сценарий в чате продолжается с вопроса о дороге.
func (b *Bot) LocationFromApp(ctx context.Context, maxUserID, name string, lat, lon float64) (string, string) {
	st := b.load(ctx, maxUserID)
	st.HasPoint, st.Lat, st.Lon, st.District, st.Address = true, lat, lon, "", ""
	in := Incoming{MaxUserID: maxUserID, Name: name}
	if st.Sport != "" && (st.Step == stepWhere || st.Step == stepTravel || st.Step == stepResults) {
		st.Step, st.Mode, st.Limit = stepTravel, "", 0
		b.save(ctx, maxUserID, st)
		b.reply(ctx, in, "📍 <b>Геолокацию получил из приложения</b>", nil)
		b.askTravel(ctx, in)
		return st.Sport, st.When
	}
	b.save(ctx, maxUserID, st)
	return st.Sport, st.When
}

func (b *Bot) askMessage(ctx context.Context, in Incoming, st state, venueID string) {
	if b.studio == nil {
		return
	}
	v, err := b.store.GetVenue(ctx, venueID)
	if err != nil {
		b.reply(ctx, in, "Не нашёл эту студию.", mc.Keyboard{{mc.Callback("В начало", "menu")}})
		return
	}
	st.Step, st.MsgVenue = stepMessage, venueID
	b.save(ctx, in.MaxUserID, st)
	b.reply(ctx, in, "✍️ <b>Сообщение студии «"+esc(v.Name)+"»</b>\n\nНапиши вопрос одним сообщением — администратор ответит, ответ придёт сюда.",
		mc.Keyboard{{mc.Callback("Отмена", "my")}})
}

func (b *Bot) sendMessage(ctx context.Context, in Incoming, st state) {
	venueID := st.MsgVenue
	st.Step, st.MsgVenue = stepMenu, ""
	b.save(ctx, in.MaxUserID, st)
	if strings.TrimSpace(in.Text) == "" {
		b.reply(ctx, in, "Пустое сообщение не отправить. Напиши текст.", mc.Keyboard{{mc.Callback("Написать ещё раз", "msg:"+venueID)}})
		return
	}
	if _, err := b.studio.SendClientMessage(ctx, in.MaxUserID, in.Name, venueID, in.Text); err != nil {
		log.Printf("bot: client message: %v", err)
		b.reply(ctx, in, "Не получилось отправить сообщение. Попробуй ещё раз.", mc.Keyboard{{mc.Callback("Написать ещё раз", "msg:"+venueID)}})
		return
	}
	b.reply(ctx, in, "📨 <b>Отправил студии</b>\nОтвет придёт сюда, в чат.", mc.Keyboard{{mc.Callback("Мои записи", "my")}})
}
