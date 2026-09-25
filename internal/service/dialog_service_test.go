package service_test

import (
	"context"
	"testing"

	"sportslot/internal/maxclient"
	"sportslot/internal/service"
)

func TestDialogService_StartAndFlow(t *testing.T) {
	ctx := context.Background()
	matchingSvc := service.NewMatchingService(&mockVenueRepo{}, &mockSlotRepo{})
	bookingSvc := service.NewBookingService(&mockBookingRepo{}, &mockSlotRepo{}, &mockUserRepo{})
	dialogSvc := service.NewDialogService(matchingSvc, bookingSvc, "http://localhost:5173")

	// 1. /start message
	resp, err := dialogSvc.HandleMessage(ctx, "user-test", "Тест", "/start")
	if err != nil {
		t.Fatalf("unexpected error on /start: %v", err)
	}
	if len(resp.Buttons) == 0 {
		t.Errorf("expected sport buttons on /start, got 0")
	}

	// 2. Select sport "boxing"
	resp, err = dialogSvc.HandleMessage(ctx, "user-test", "Тест", "sport:boxing")
	if err != nil {
		t.Fatalf("unexpected error on sport selection: %v", err)
	}
	if resp.Text != maxclient.MsgAskDate {
		t.Errorf("expected MsgAskDate, got: %s", resp.Text)
	}

	// 3. Answer date
	resp, err = dialogSvc.HandleMessage(ctx, "user-test", "Тест", "завтра")
	if err != nil {
		t.Fatalf("unexpected error on date: %v", err)
	}
	if resp.Text != maxclient.MsgAskArea {
		t.Errorf("expected MsgAskArea, got: %s", resp.Text)
	}
}
