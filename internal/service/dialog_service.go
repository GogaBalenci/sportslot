package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"sportslot/internal/maxclient"
	"sportslot/internal/model"
)

type DialogStep string

const (
	StepAskSport    DialogStep = "ask_sport"
	StepAskDate     DialogStep = "ask_date"
	StepAskArea     DialogStep = "ask_area"
	StepShowResults DialogStep = "show_results"
)

type DialogState struct {
	Step        DialogStep
	SportType   string
	DateFrom    time.Time
	DateTo      time.Time
	Lat         float64
	Lon         float64
	LastResults []model.VenueSlotResult
	UpdatedAt   time.Time
}

type DialogResponse struct {
	Text        string
	Buttons     []maxclient.MessageButton
	MiniAppLink string
}

// DialogService - машина состояний диалога чат-бота: выбор спорта ->
// даты -> района -> показ вариантов -> бронь. Состояние хранится в памяти
// на ключ max_user_id (MVP; для продакшена - вынести в Redis/БД).
type DialogService struct {
	mu         sync.Mutex
	states     map[string]*DialogState
	matching   *MatchingService
	booking    *BookingService
	miniAppURL string
}

func NewDialogService(matching *MatchingService, booking *BookingService, miniAppURL string) *DialogService {
	return &DialogService{
		states:     make(map[string]*DialogState),
		matching:   matching,
		booking:    booking,
		miniAppURL: miniAppURL,
	}
}

func (d *DialogService) getState(maxUserID string) DialogState {
	d.mu.Lock()
	defer d.mu.Unlock()

	st, ok := d.states[maxUserID]
	if !ok {
		st = &DialogState{Step: StepAskSport, UpdatedAt: time.Now()}
		d.states[maxUserID] = st
	}
	return *st
}

// saveState is the only method that writes to states. The mutex deliberately
// protects the map only; callers work with value copies outside the lock.
func (d *DialogService) saveState(maxUserID string, state DialogState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.states[maxUserID] = &state
}

func (d *DialogService) resetState(maxUserID string) {
	d.saveState(maxUserID, DialogState{Step: StepAskSport, UpdatedAt: time.Now()})
}

// HandleMessage - основная точка входа диалогового sceneграфа.
func (d *DialogService) HandleMessage(ctx context.Context, maxUserID, userName, text string) (*DialogResponse, error) {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)

	if lower == "/start" || lower == "начать" {
		d.resetState(maxUserID)
		return &DialogResponse{
			Text:    maxclient.MsgWelcome,
			Buttons: maxclient.SportTypeButtons(),
		}, nil
	}

	state := d.getState(maxUserID)

	switch state.Step {
	case StepAskSport:
		sport := parseSportPayload(text)
		if sport == "" {
			return &DialogResponse{
				Text:    maxclient.MsgWelcome,
				Buttons: maxclient.SportTypeButtons(),
			}, nil
		}
		state.SportType = sport
		state.Step = StepAskDate
		state.UpdatedAt = time.Now()
		d.saveState(maxUserID, state)
		return &DialogResponse{Text: maxclient.MsgAskDate}, nil

	case StepAskDate:
		dateFrom, dateTo := parseDateRange(text)
		state.DateFrom = dateFrom
		state.DateTo = dateTo
		state.Step = StepAskArea
		state.UpdatedAt = time.Now()
		d.saveState(maxUserID, state)
		return &DialogResponse{Text: maxclient.MsgAskArea}, nil

	case StepAskArea:
		lat, lon := parseArea(text)
		state.Lat = lat
		state.Lon = lon
		state.Step = StepShowResults
		state.UpdatedAt = time.Now()
		d.saveState(maxUserID, state)
		return d.showResults(ctx, maxUserID, &state)

	case StepShowResults:
		return d.handleBookingChoice(ctx, maxUserID, userName, text, &state)

	default:
		d.resetState(maxUserID)
		return &DialogResponse{
			Text:    maxclient.MsgWelcome,
			Buttons: maxclient.SportTypeButtons(),
		}, nil
	}
}

func (d *DialogService) showResults(ctx context.Context, maxUserID string, state *DialogState) (*DialogResponse, error) {
	results, err := d.matching.FindSlots(ctx, SearchCriteria{
		SportType: state.SportType,
		Lat:       state.Lat,
		Lon:       state.Lon,
		RadiusKM:  5,
		DateFrom:  state.DateFrom,
		DateTo:    state.DateTo,
	})
	if err != nil {
		return nil, fmt.Errorf("find slots: %w", err)
	}

	state.LastResults = results
	d.saveState(maxUserID, *state)

	if len(results) == 0 {
		d.resetState(maxUserID)
		return &DialogResponse{Text: maxclient.MsgNoResults}, nil
	}

	if len(results) > 3 {
		results = results[:3]
	}

	var sb strings.Builder
	sb.WriteString("Нашёл подходящие варианты:\n\n")

	buttons := make([]maxclient.MessageButton, 0, len(results))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf(
			"%d. %s (%s)\n%s, свободно мест: %d\n\n",
			i+1, r.Name, r.Address, r.Slot.StartAt.Format("02.01 15:04"), r.Slot.QuotaAvailable,
		))
		buttons = append(buttons, maxclient.MessageButton{
			Text:    fmt.Sprintf("Забронировать №%d", i+1),
			Payload: fmt.Sprintf("book:%s", r.Slot.SlotID),
		})
	}

	resp := &DialogResponse{
		Text:    sb.String(),
		Buttons: buttons,
	}

	if d.miniAppURL != "" {
		params := url.Values{}
		params.Set("sport", state.SportType)
		params.Set("user", maxUserID)
		resp.MiniAppLink = d.miniAppURL + "?" + params.Encode()
	}

	return resp, nil
}

func (d *DialogService) handleBookingChoice(ctx context.Context, maxUserID, userName, text string, state *DialogState) (*DialogResponse, error) {
	slotID := parseBookingPayload(text)
	if slotID == "" {
		return &DialogResponse{Text: maxclient.MsgUnknownChoice}, nil
	}

	_, err := d.booking.CreateBooking(ctx, CreateBookingInput{
		MaxUserID:     maxUserID,
		UserName:      userName,
		SlotID:        slotID,
		SourceChannel: model.SourceChannelBot,
	})
	if err != nil {
		if err == ErrQuotaExceeded {
			resp, showErr := d.showResults(ctx, maxUserID, state)
			if showErr != nil {
				return nil, showErr
			}
			resp.Text = maxclient.MsgQuotaExceeded + "\n\n" + resp.Text
			return resp, nil
		}
		return nil, fmt.Errorf("create booking: %w", err)
	}

	d.resetState(maxUserID)
	return &DialogResponse{Text: maxclient.MsgBookingConfirmed}, nil
}

func parseSportPayload(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.TrimPrefix(text, "sport:")
	switch text {
	case "boxing", "бокс":
		return "boxing"
	case "yoga", "йога":
		return "yoga"
	case "football", "футбол":
		return "football"
	default:
		return ""
	}
}

// parseDateRange - упрощённый парсинг MVP: любое сообщение трактуется как
// "ближайшие 7 дней". В продакшене здесь должен быть парсинг конкретных
// дат/диапазона из текста или из структурированного payload'а MAX.
func parseDateRange(text string) (time.Time, time.Time) {
	_ = text
	now := time.Now()
	return now, now.Add(7 * 24 * time.Hour)
}

// parseArea - упрощённый парсинг MVP: координаты центра Москвы по умолчанию,
// если пользователь не поделился геолокацией через MAX Bridge.
func parseArea(text string) (float64, float64) {
	_ = text
	return 55.751244, 37.618423
}

func parseBookingPayload(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "book:") {
		return strings.TrimPrefix(text, "book:")
	}
	return ""
}
