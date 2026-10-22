// Команда ops — мониторинг и резервные копии СпортСлота. Работает отдельным
// контейнером: проверяет API и сайт, пишет администраторам платформы в MAX
// при падении и восстановлении, раз в сутки делает pg_dump и хранит копии
// заданное число дней.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sportslot/internal/maxclient"
	"sportslot/internal/repository/postgres"
)

type config struct {
	checkURLs  []string
	interval   time.Duration
	failLimit  int
	adminIDs   []string
	dbURL      string
	backupDir  string
	backupHour int
	keepDays   int
	minFreeMB  int64
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	n, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func list(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	cfg := config{
		checkURLs:  list(env("OPS_CHECK_URLS", "http://api:8080/api/v1/health")),
		interval:   time.Duration(envInt("OPS_INTERVAL_SECONDS", 60)) * time.Second,
		failLimit:  envInt("OPS_FAILS_BEFORE_ALERT", 3),
		adminIDs:   list(os.Getenv("PLATFORM_ADMIN_IDS")),
		dbURL:      os.Getenv("DATABASE_URL"),
		backupDir:  env("BACKUP_DIR", "/backups"),
		backupHour: envInt("BACKUP_HOUR", 4),
		keepDays:   envInt("BACKUP_KEEP_DAYS", 14),
		minFreeMB:  int64(envInt("OPS_MIN_FREE_MB", 1024)),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	o := &ops{cfg: cfg, client: maxclient.NewClient(env("MAX_BOT_API_BASE_URL", "https://platform-api2.max.ru"), os.Getenv("MAX_BOT_API_TOKEN")),
		http: &http.Client{Timeout: 10 * time.Second}, failures: map[string]int{}, downSince: map[string]time.Time{}}
	if cfg.dbURL != "" {
		if pool, err := postgres.NewPool(ctx, cfg.dbURL); err == nil {
			o.store = postgres.NewStore(pool)
			defer pool.Close()
		} else {
			log.Printf("ops: db: %v (status is not saved)", err)
		}
	}
	if !o.client.Enabled() || len(cfg.adminIDs) == 0 {
		log.Println("ops: MAX_BOT_API_TOKEN or PLATFORM_ADMIN_IDS is empty: alerts go only to the log")
	}
	log.Printf("ops: checking %v every %s, backup at %02d:00 to %s, keep %d days", cfg.checkURLs, cfg.interval, cfg.backupHour, cfg.backupDir, cfg.keepDays)
	o.alert(ctx, fmt.Sprintf("👀 <b>Мониторинг СпортСлот запущен</b>\nПроверяю %d %s раз в %s, бэкап базы — каждый день в %02d:00, храню %d дней.",
		len(cfg.checkURLs), plural(len(cfg.checkURLs), "адрес", "адреса", "адресов"), cfg.interval, cfg.backupHour, cfg.keepDays))

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	o.tick(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			o.tick(ctx, now)
		}
	}
}

type ops struct {
	cfg       config
	client    *maxclient.Client
	store     *postgres.Store
	http      *http.Client
	failures  map[string]int
	downSince map[string]time.Time
	lastDump  string // дата последнего бэкапа, YYYY-MM-DD
	diskWarn  string // дата последнего предупреждения о диске
}

func (o *ops) tick(ctx context.Context, now time.Time) {
	allOK := true
	for _, url := range o.cfg.checkURLs {
		if !o.check(ctx, url, now) {
			allOK = false
		}
	}
	state := "ok"
	if !allOK {
		state = "есть сбои"
	}
	o.status(ctx, "watchdog", state)

	day := now.Format("2006-01-02")
	if now.Hour() >= o.cfg.backupHour && o.lastDump != day && o.cfg.dbURL != "" {
		o.lastDump = day
		o.backup(ctx, now)
	}
	o.checkDisk(ctx, day)
}

// check — один адрес. Тревога после нескольких неудач подряд, чтобы не
// будить из-за секундного перезапуска; при восстановлении — сообщение с длительностью простоя.
func (o *ops) check(ctx context.Context, url string, now time.Time) bool {
	err := o.probe(ctx, url)
	if err == nil {
		if o.failures[url] >= o.cfg.failLimit {
			o.alert(ctx, fmt.Sprintf("🟢 <b>СпортСлот снова работает</b>\n%s\nПростой: %s", esc(url), duration(now.Sub(o.downSince[url]))))
		}
		o.failures[url] = 0
		delete(o.downSince, url)
		return true
	}
	if o.failures[url] == 0 {
		o.downSince[url] = now
	}
	o.failures[url]++
	log.Printf("ops: %s: %v (%d)", url, err, o.failures[url])
	if o.failures[url] == o.cfg.failLimit {
		o.alert(ctx, fmt.Sprintf("🔴 <b>СпортСлот недоступен</b>\n%s\n%s\nС %s, %d проверки подряд.",
			esc(url), esc(err.Error()), o.downSince[url].Format("15:04"), o.cfg.failLimit))
	}
	return false
}

func (o *ops) probe(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return fmt.Errorf("нет ответа: %v", shortErr(err))
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ответ %d", resp.StatusCode)
	}
	return nil
}

func (o *ops) backup(ctx context.Context, now time.Time) {
	if err := os.MkdirAll(o.cfg.backupDir, 0o750); err != nil {
		o.backupFailed(ctx, err)
		return
	}
	name := filepath.Join(o.cfg.backupDir, "sportslot-"+now.Format("2006-01-02")+".dump")
	tmp := name + ".part"
	dumpCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(dumpCtx, "pg_dump", "--format=custom", "--no-owner", "--file="+tmp, o.cfg.dbURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp)
		o.backupFailed(ctx, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out))))
		return
	}
	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		os.Remove(tmp)
		o.backupFailed(ctx, fmt.Errorf("пустой файл дампа"))
		return
	}
	if err := os.Rename(tmp, name); err != nil {
		o.backupFailed(ctx, err)
		return
	}
	removed := o.rotate(now)
	log.Printf("ops: backup %s (%s), removed %d old", name, size(info.Size()), removed)
	o.status(ctx, "backup_ok", fmt.Sprintf("%s, %s", filepath.Base(name), size(info.Size())))
}

func (o *ops) backupFailed(ctx context.Context, err error) {
	log.Printf("ops: backup failed: %v", err)
	o.status(ctx, "backup_error", shortErr(err))
	o.alert(ctx, "⚠️ <b>Бэкап базы не сделан</b>\n"+esc(shortErr(err))+"\nПопробую снова завтра; можно запустить вручную: docker compose restart ops.")
}

// rotate удаляет копии старше keepDays, но всегда оставляет три последние.
func (o *ops) rotate(now time.Time) int {
	files, _ := filepath.Glob(filepath.Join(o.cfg.backupDir, "sportslot-*.dump"))
	sort.Strings(files)
	removed := 0
	for i, f := range files {
		if len(files)-i <= 3 {
			break
		}
		info, err := os.Stat(f)
		if err == nil && now.Sub(info.ModTime()) > time.Duration(o.cfg.keepDays)*24*time.Hour {
			if os.Remove(f) == nil {
				removed++
			}
		}
	}
	return removed
}

func (o *ops) checkDisk(ctx context.Context, day string) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(o.cfg.backupDir, &st); err != nil {
		return
	}
	freeMB := int64(st.Bavail) * int64(st.Bsize) / (1 << 20)
	if freeMB < o.cfg.minFreeMB && o.diskWarn != day {
		o.diskWarn = day
		o.alert(ctx, fmt.Sprintf("💽 <b>Мало места на сервере</b>: свободно %d МБ. Удалите старые образы: docker image prune -f", freeMB))
	}
}

func (o *ops) status(ctx context.Context, key, value string) {
	if o.store == nil {
		return
	}
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := o.store.SetOpsStatus(sctx, key, value); err != nil && key != "watchdog" {
		log.Printf("ops: save status %s: %v", key, err)
	}
}

func (o *ops) alert(ctx context.Context, text string) {
	log.Printf("ops alert: %s", strings.ReplaceAll(text, "\n", " | "))
	if !o.client.Enabled() {
		return
	}
	for _, id := range o.cfg.adminIDs {
		sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := o.client.SendMessage(sctx, id, text, nil); err != nil {
			log.Printf("ops: alert to %s: %v", id, err)
		}
		cancel()
	}
}

func duration(d time.Duration) string {
	m := int(d.Round(time.Minute).Minutes())
	if m < 1 {
		return "меньше минуты"
	}
	if m < 60 {
		return fmt.Sprintf("%d мин", m)
	}
	return fmt.Sprintf("%d ч %d мин", m/60, m%60)
}

func size(b int64) string {
	if b < 1<<20 {
		return fmt.Sprintf("%d КБ", b>>10)
	}
	return fmt.Sprintf("%.1f МБ", float64(b)/(1<<20))
}

func shortErr(err error) string {
	s := err.Error()
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}

func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func plural(n int, one, few, many string) string {
	n %= 100
	if n >= 11 && n <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}
