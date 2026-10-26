package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	stOn    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	stOff   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)
	stDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stSel   = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	stErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	stOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	stWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	stTitle = lipgloss.NewStyle().Bold(true)
	stKey   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type screen int

const (
	scList screen = iota
	scInput
	scLog
	scDirect
	scInfo
)

type (
	tickMsg   struct{}
	statusMsg struct {
		running  bool
		st       *State // свежее состояние с диска (его мог поменять vpn watch)
		up, down int64
		statsOK  bool
		at       time.Time
	}
	opMsg struct {
		text      string
		err       error
		st        *State
		keepPings bool
	}
	pingMsg struct {
		res []PingResult
		err error
	}
)

type model struct {
	st      *State
	scr     screen
	cursor  int
	offset  int
	pings   []PingResult
	running bool
	busy    string
	frame   int
	msg     string
	msgErr  bool
	w, h    int

	// ввод: подписка или исключение
	inputFor string // "sub" | "direct"
	inputBuf string

	// экраны
	logLines  []string
	logOffset int
	dirCursor int

	// скорость
	lastUp, lastDown int64
	lastAt           time.Time
	speedUp, speedDn float64
	sessUp, sessDn   int64
	statsOK          bool
}

func runTUI(st *State) error {
	m := &model{st: st, w: 80, h: 24}
	if st.Selected >= 0 {
		m.cursor = st.Selected
	}
	m.running, _ = isRunning()
	if len(st.All()) == 0 {
		m.openInput("sub", st.SubURL)
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(*model); ok && fm.running {
		fmt.Println(L("VPN продолжает работать в фоне. Выключить: vpn off", "The VPN keeps running in the background. Stop it with: vpn off"))
	}
	return nil
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), checkStatus(m.st)}
	if subUpdateDue(m.st) && m.scr == scList {
		cmds = append(cmds, m.start(L("Обновляю подписку", "Updating subscription"), opAutoUpdate(m.copyState())))
	}
	return tea.Batch(cmds...)
}

func opAutoUpdate(st *State) tea.Cmd {
	return func() tea.Msg {
		msg, err := autoUpdateSub(st)
		if err != nil {
			return opMsg{err: err}
		}
		return opMsg{text: msg, st: st}
	}
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// checkStatus раз в секунду: жив ли xray, свежий state.json и счётчики трафика.
func checkStatus(cur *State) tea.Cmd {
	return func() tea.Msg {
		msg := statusMsg{at: time.Now()}
		msg.running, _ = isRunning()
		if st, err := loadState(); err == nil {
			msg.st = st
			cur = st
		}
		if msg.running {
			msg.up, msg.down, msg.statsOK = readTraffic(cur.MetricsPort())
		}
		return msg
	}
}

func readTraffic(port int) (up, down int64, ok bool) {
	cl := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := cl.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/vars", port))
	if err != nil {
		return 0, 0, false
	}
	defer resp.Body.Close()
	var v struct {
		Stats struct {
			Outbound map[string]struct {
				Uplink   int64 `json:"uplink"`
				Downlink int64 `json:"downlink"`
			} `json:"outbound"`
		} `json:"stats"`
	}
	if json.NewDecoder(resp.Body).Decode(&v) != nil {
		return 0, 0, false
	}
	p, ok := v.Stats.Outbound["proxy"]
	return p.Uplink, p.Downlink, ok
}

func (m *model) start(label string, cmd tea.Cmd) tea.Cmd {
	m.busy, m.msg = label, ""
	return cmd
}

func (m *model) setMsg(text string, isErr bool) { m.msg, m.msgErr = text, isErr }

// copyState — фоновые операции работают с копией, чтобы не гоняться с отрисовкой.
func (m *model) copyState() *State {
	cp := *m.st
	cp.Direct = append([]string(nil), m.st.Direct...)
	return &cp
}

func (m *model) openInput(purpose, initial string) {
	m.scr, m.inputFor, m.inputBuf = scInput, purpose, initial
}

func opOn(st *State) tea.Cmd {
	return func() tea.Msg {
		if err := vpnOn(st, func(string) {}); err != nil {
			return opMsg{err: err}
		}
		p, _ := st.Current()
		return opMsg{text: L("Включено: ", "Connected: ") + p.Name, st: st, keepPings: true}
	}
}

func opOff(st *State) tea.Cmd {
	return func() tea.Msg {
		text := L("Выключено", "Disconnected")
		if err := vpnOff(st); err != nil {
			text += L(" (предупреждение: ", " (warning: ") + err.Error() + ")"
		}
		return opMsg{text: text, st: st, keepPings: true}
	}
}

func opUpdate(st *State) tea.Cmd {
	return func() tea.Msg {
		n, skipped, err := updateSub(st)
		if err != nil {
			return opMsg{err: err}
		}
		t := fmt.Sprintf(L("Подписка обновлена: %d серверов", "Subscription updated: %d servers"), n)
		if len(skipped) > 0 {
			t += fmt.Sprintf(L(", пропущено %d", ", %d skipped"), len(skipped))
		}
		st.Save()
		return opMsg{text: t, st: st}
	}
}

func opPing(st *State, real bool) tea.Cmd {
	return func() tea.Msg {
		if real {
			res, err := realPing(st, st.All(), func(string) {})
			return pingMsg{res, err}
		}
		return pingMsg{res: tcpPing(st.All())}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		m.frame++
		cmds := []tea.Cmd{tick()}
		if m.frame%10 == 0 && m.busy == "" {
			cmds = append(cmds, checkStatus(m.st))
		}
		return m, tea.Batch(cmds...)
	case statusMsg:
		m.applyStatus(msg)
	case opMsg:
		m.busy = ""
		if msg.st != nil {
			m.st = msg.st
			if !msg.keepPings {
				m.pings = nil
			}
			m.clampCursor()
		}
		if msg.err != nil {
			m.setMsg(msg.err.Error(), true)
		} else {
			m.setMsg(msg.text, false)
		}
		return m, checkStatus(m.st)
	case pingMsg:
		m.busy = ""
		if msg.err != nil {
			m.setMsg(msg.err.Error(), true)
		} else {
			m.pings = msg.res
			alive := 0
			for _, r := range msg.res {
				if r.Delay > 0 {
					alive++
				}
			}
			m.setMsg(fmt.Sprintf(L("Отвечают %d из %d · b — перейти к самому быстрому", "%d of %d responding · b — jump to the fastest"), alive, len(msg.res)), false)
		}
	case tea.KeyMsg:
		switch m.scr {
		case scInput:
			return m.updateInput(msg)
		case scLog:
			return m.updateLog(msg)
		case scDirect:
			return m.updateDirect(msg)
		case scInfo:
			if s := msg.String(); s == "esc" || s == "q" || s == "i" || s == "enter" {
				m.scr = scList
			}
			return m, nil
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m *model) applyStatus(msg statusMsg) {
	wasRunning := m.running
	m.running = msg.running
	// Подхватываем изменения с диска (например, vpn watch переключил сервер),
	// но не посреди ввода — чтобы не сбить редактирование.
	if msg.st != nil && m.busy == "" && m.scr != scInput {
		if len(msg.st.All()) != len(m.st.All()) {
			m.pings = nil
		}
		if msg.st.Selected != m.st.Selected && msg.st.Selected >= 0 && m.running {
			if p, ok := msg.st.Current(); ok {
				m.setMsg(L("Сервер переключён: ", "Server switched: ")+p.Name, false)
			}
		}
		m.st = msg.st
		m.clampCursor()
	}
	if m.scr == scLog {
		m.loadLog(false)
	}
	m.statsOK = msg.statsOK
	if !msg.running || !msg.statsOK {
		m.speedUp, m.speedDn = 0, 0
		m.lastAt = time.Time{}
		return
	}
	if !wasRunning || m.lastAt.IsZero() || msg.up < m.lastUp || msg.down < m.lastDown {
		m.lastUp, m.lastDown, m.lastAt = msg.up, msg.down, msg.at // xray перезапустился — счётчики с нуля
	} else if dt := msg.at.Sub(m.lastAt).Seconds(); dt > 0.2 {
		m.speedUp = float64(msg.up-m.lastUp) / dt
		m.speedDn = float64(msg.down-m.lastDown) / dt
		m.lastUp, m.lastDown, m.lastAt = msg.up, msg.down, msg.at
	}
	m.sessUp, m.sessDn = msg.up, msg.down
}

func (m *model) clampCursor() {
	if n := len(m.st.All()); m.cursor >= n {
		m.cursor = max(n-1, 0)
	}
	if m.dirCursor >= len(m.st.Direct) {
		m.dirCursor = max(len(m.st.Direct)-1, 0)
	}
}

func (m *model) updateInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		if m.inputFor == "sub" && len(m.st.All()) == 0 {
			return m, tea.Quit
		}
		m.scr = scList
		if m.inputFor == "direct" {
			m.scr = scDirect
		}
	case tea.KeyEnter:
		v := strings.TrimSpace(m.inputBuf)
		if m.inputFor == "direct" {
			m.scr = scDirect
			if v == "" {
				return m, nil
			}
			m.st.Direct = append(m.st.Direct, strings.Fields(v)...)
			m.dirCursor = len(m.st.Direct) - 1
			return m, m.saveDirect(L("Исключение добавлено", "Exclusion added"))
		}
		if !strings.HasPrefix(v, "http") {
			m.setMsg(L("Нужна ссылка вида https://…", "Expected a URL like https://…"), true)
			return m, nil
		}
		m.scr = scList
		m.st.SubURL = v
		m.st.Save()
		return m, m.start(L("Скачиваю подписку", "Downloading subscription"), opUpdate(m.copyState()))
	case tea.KeyBackspace:
		if r := []rune(m.inputBuf); len(r) > 0 {
			m.inputBuf = string(r[:len(r)-1])
		}
	case tea.KeyCtrlU:
		m.inputBuf = ""
	case tea.KeySpace:
		m.inputBuf += " "
	case tea.KeyRunes: // вставка из буфера тоже приходит сюда
		m.inputBuf += string(k.Runes)
	}
	return m, nil
}

func (m *model) saveDirect(text string) tea.Cmd {
	m.st.Save()
	if m.running {
		return m.start(L("Применяю исключения", "Applying exclusions"), opOn(m.copyState()))
	}
	m.setMsg(text, false)
	return nil
}

func (m *model) updateDirect(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy != "" {
		return m, nil
	}
	switch k.String() {
	case "esc", "q", "d":
		m.scr = scList
	case "up", "k":
		if m.dirCursor > 0 {
			m.dirCursor--
		}
	case "down", "j":
		if m.dirCursor < len(m.st.Direct)-1 {
			m.dirCursor++
		}
	case "a", "+", "enter":
		m.openInput("direct", "")
	case "x", "delete", "backspace", "-":
		if len(m.st.Direct) == 0 {
			return m, nil
		}
		i := m.dirCursor
		m.st.Direct = append(m.st.Direct[:i:i], m.st.Direct[i+1:]...)
		m.clampCursor()
		return m, m.saveDirect(L("Исключение удалено", "Exclusion removed"))
	}
	return m, nil
}

func (m *model) loadLog(toEnd bool) {
	var lines []string
	if b, err := os.ReadFile(watchLogPath()); err == nil && len(b) > 0 {
		lines = append(lines, L("── сторож (vpn watch) ──", "── watcher (vpn watch) ──"))
		lines = append(lines, strings.Split(lastLines(string(b), 15), "\n")...)
		lines = append(lines, "")
	}
	lines = append(lines, "── xray ──")
	if l := tailLog(300); l != "" {
		lines = append(lines, strings.Split(l, "\n")...)
	} else {
		lines = append(lines, L("(лог пуст — VPN ещё не включали)", "(log is empty — the VPN hasn't been started yet)"))
	}
	follow := toEnd || m.logOffset >= max(len(m.logLines)-m.logRows(), 0)
	m.logLines = lines
	if follow {
		m.logOffset = max(len(lines)-m.logRows(), 0)
	}
}

func (m *model) logRows() int { return max(m.h-6, 3) }

func (m *model) updateLog(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxOff := max(len(m.logLines)-m.logRows(), 0)
	switch k.String() {
	case "esc", "q", "l":
		m.scr = scList
	case "up", "k":
		m.logOffset = max(m.logOffset-1, 0)
	case "down", "j":
		m.logOffset = min(m.logOffset+1, maxOff)
	case "pgup":
		m.logOffset = max(m.logOffset-m.logRows(), 0)
	case "pgdown", " ":
		m.logOffset = min(m.logOffset+m.logRows(), maxOff)
	case "G", "end":
		m.logOffset = maxOff
	case "g", "home":
		m.logOffset = 0
	}
	return m, nil
}

func (m *model) updateKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.st.All())
	key := k.String()
	if m.busy != "" && key != "q" && key != "ctrl+c" {
		return m, nil // пока идёт операция, игнорируем остальное
	}
	switch key {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < n-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(n-1, 0)
	case "enter":
		if n == 0 {
			return m, nil
		}
		m.st.Selected = m.cursor
		m.st.Save()
		return m, m.start(L("Подключаюсь", "Connecting"), opOn(m.copyState()))
	case " ", "o":
		if m.running {
			return m, m.start(L("Выключаю", "Disconnecting"), opOff(m.copyState()))
		}
		if _, ok := m.st.Current(); !ok && n > 0 {
			m.st.Selected = m.cursor
			m.st.Save()
		}
		return m, m.start(L("Подключаюсь", "Connecting"), opOn(m.copyState()))
	case "p":
		if n > 0 {
			return m, m.start(L("TCP-пинг", "TCP ping"), opPing(m.copyState(), false))
		}
	case "P":
		if n > 0 {
			return m, m.start(L("Реальный пинг через каждый сервер", "Real ping through every server"), opPing(m.copyState(), true))
		}
	case "b":
		if m.pings == nil {
			m.setMsg(L("Сначала пинг: p или P", "Ping first: p or P"), true)
			return m, nil
		}
		best := -1
		for i, r := range m.pings {
			if r.Delay > 0 && (best < 0 || r.Delay < m.pings[best].Delay) {
				best = i
			}
		}
		if best >= 0 {
			m.cursor = best
			m.setMsg(L("Самый быстрый — Enter, чтобы подключиться", "Fastest server — press Enter to connect"), false)
		}
	case "u":
		if m.st.SubURL == "" {
			m.openInput("sub", "")
			return m, nil
		}
		return m, m.start(L("Обновляю подписку", "Updating subscription"), opUpdate(m.copyState()))
	case "s":
		m.openInput("sub", m.st.SubURL)
	case "m":
		if m.st.Mode == "ru" {
			m.st.Mode = "global"
		} else {
			m.st.Mode = "ru"
		}
		m.st.Save()
		if m.running {
			return m, m.start(L("Применяю режим", "Applying mode"), opOn(m.copyState()))
		}
		m.setMsg(L("Режим: ", "Mode: ")+m.st.Mode, false)
	case "l":
		m.scr = scLog
		m.loadLog(true)
	case "d":
		m.scr = scDirect
	case "i":
		if n > 0 {
			m.scr = scInfo
		}
	}
	return m, nil
}

// ---------- отрисовка ----------

func (m *model) View() string {
	var b strings.Builder
	w := max(m.w, 40)
	b.WriteString(m.header(w))
	switch m.scr {
	case scInput:
		m.viewInput(&b, w)
	case scLog:
		m.viewLog(&b, w)
	case scDirect:
		m.viewDirect(&b, w)
	case scInfo:
		m.viewInfo(&b, w)
	default:
		m.viewList(&b, w)
	}
	b.WriteString(m.footerMsg())
	return b.String()
}

func (m *model) header(w int) string {
	status := stOff.Render(L("○ ВЫКЛ", "○ OFF"))
	if m.running {
		status = stOn.Render(L("● ВКЛ", "● ON"))
	}
	cur := L("сервер не выбран", "no server selected")
	if p, ok := m.st.Current(); ok {
		cur = p.Name
	}
	right := L("режим ", "mode ") + m.st.Mode
	if autostartEnabled() {
		right += L(" · автозапуск", " · autostart")
	}
	line1 := fmt.Sprintf(" %s  %s  %s", status, stTitle.Render(trunc(cur, w-36)), stDim.Render(right))
	var info string
	if m.running && m.statsOK {
		info = fmt.Sprintf(L(" ↑ %s  ↓ %s  · за сессию %s", " ↑ %s  ↓ %s  · session %s"), fmtRate(m.speedUp), fmtRate(m.speedDn), fmtSize(m.sessUp+m.sessDn))
	} else {
		info = fmt.Sprintf(L(" SOCKS :%d · HTTP :%d · SSH напрямую", " SOCKS :%d · HTTP :%d · SSH direct"), m.st.SocksPort, m.st.HTTPPort)
	}
	if l := subLine(m.st); l != "" {
		info += " · " + l
	}
	return line1 + "\n" + stDim.Render(trunc(info, w)) + "\n\n"
}

func (m *model) viewInput(b *strings.Builder, w int) {
	if m.inputFor == "direct" {
		b.WriteString(L(" Домен, IP или подсеть, которые пойдут мимо VPN (можно несколько через пробел):\n", " Domain, IP or subnet to bypass the VPN (several separated by spaces):\n"))
		b.WriteString(stDim.Render(L(" например: myserver.com  203.0.113.7  10.8.0.0/24", " e.g. myserver.com  203.0.113.7  10.8.0.0/24")) + "\n\n")
	} else {
		b.WriteString(L(" Вставьте ссылку на подписку от хостера (https://…) и нажмите Enter:\n\n", " Paste your subscription URL (https://…) and press Enter:\n\n"))
	}
	b.WriteString(" > " + trunc(m.inputBuf, w-5) + "█\n\n")
	b.WriteString(stDim.Render(L(" Enter — сохранить · Ctrl+U — очистить · Esc — отмена", " Enter — save · Ctrl+U — clear · Esc — cancel")) + "\n")
}

func (m *model) viewList(b *strings.Builder, w int) {
	all := m.st.All()
	rows := max(m.h-7, 3)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if len(all) == 0 {
		b.WriteString(L(" Серверов нет. s — задать подписку.\n", " No servers. Press s to set a subscription.\n"))
	}
	for i := m.offset; i < len(all) && i < m.offset+rows; i++ {
		p := all[i]
		mark := "  "
		if i == m.st.Selected {
			mark = "▶ "
		}
		delay := ""
		if m.pings != nil && i < len(m.pings) {
			delay = colorDelay(m.pings[i])
		}
		line := fmt.Sprintf("%s%3d  %-20s ", mark, i+1, trunc(p.Summary(), 20))
		name := trunc(p.Name, w-lipgloss.Width(line)-10)
		row := line + fmt.Sprintf("%-*s", max(w-lipgloss.Width(line)-9, 0), name)
		if i == m.cursor {
			row = stSel.Render(row)
		}
		b.WriteString(row + " " + delay + "\n")
	}
	if len(all) > rows {
		b.WriteString(stDim.Render(fmt.Sprintf(L(" … %d–%d из %d", " … %d–%d of %d"), m.offset+1, min(m.offset+rows, len(all)), len(all))) + "\n")
	}
	b.WriteString("\n" + keys(w, "Enter", L("подключить", "connect"), "Space", L("вкл/выкл", "on/off"), "p/P", L("пинг", "ping"), "b", L("лучший", "fastest"),
		"u", L("обновить", "update"), "m", L("режим", "mode"), "d", L("исключения", "exclusions"), "i", L("инфо", "info"), "l", L("лог", "log"), "q", L("выход", "quit")) + "\n")
}

func (m *model) viewLog(b *strings.Builder, w int) {
	rows := m.logRows()
	end := min(m.logOffset+rows, len(m.logLines))
	for _, l := range m.logLines[min(m.logOffset, end):end] {
		b.WriteString(" " + trunc(l, w-2) + "\n")
	}
	for i := end - m.logOffset; i < rows; i++ {
		b.WriteString("\n")
	}
	b.WriteString(keys(w, "↑↓", L("листать", "scroll"), "PgUp/PgDn", L("страница", "page"), "G", L("в конец", "end"), "Esc", L("назад", "back")) + "\n")
}

func (m *model) viewDirect(b *strings.Builder, w int) {
	b.WriteString(stTitle.Render(L(" Мимо VPN всегда идут:", " Always bypassing the VPN:")) + stDim.Render(L(" SSH (порт 22), локальные сети, localhost", " SSH (port 22), local networks, localhost")) + "\n\n")
	if len(m.st.Direct) == 0 {
		b.WriteString(stDim.Render(L(" Своих исключений нет. a — добавить.", " No custom exclusions. Press a to add.")) + "\n")
	}
	for i, d := range m.st.Direct {
		row := fmt.Sprintf("  %-*s", w-4, trunc(d, w-4))
		if i == m.dirCursor {
			row = stSel.Render(row)
		}
		b.WriteString(row + "\n")
	}
	b.WriteString("\n" + keys(w, "a", L("добавить", "add"), "x", L("удалить", "remove"), "Esc", L("назад", "back")) + "\n")
}

func (m *model) viewInfo(b *strings.Builder, w int) {
	all := m.st.All()
	if m.cursor >= len(all) {
		return
	}
	p := all[m.cursor]
	row := func(k, v string) {
		if v != "" {
			b.WriteString(fmt.Sprintf(" %-14s %s\n", stDim.Render(k), trunc(v, w-17)))
		}
	}
	b.WriteString(stTitle.Render(" "+trunc(p.Name, w-2)) + "\n\n")
	row(L("Протокол", "Protocol"), p.Summary())
	row(L("Адрес", "Address"), p.Endpoint())
	row(L("Транспорт", "Transport"), p.Network)
	row(L("Путь", "Path"), p.Path)
	row("gRPC service", p.ServiceName)
	row("Host", p.Host)
	row(L("Режим", "Mode"), p.Mode)
	row(L("Безопасность", "Security"), p.Security)
	row("SNI", p.SNI)
	row("Fingerprint", p.Fingerprint)
	row("Flow", p.Flow)
	row("UUID", mask(p.ID))
	row(L("Пароль", "Password"), mask(p.Password))
	row("Public key", mask(p.PublicKey))
	row("Short ID", mask(p.ShortID))
	if m.pings != nil && m.cursor < len(m.pings) {
		r := m.pings[m.cursor]
		if r.Delay < 0 && r.Err != nil {
			row(L("Пинг", "Ping"), L("не отвечает: ", "not responding: ")+shortErr(r.Err))
		} else {
			row(L("Пинг", "Ping"), fmtDelay(r))
		}
	}
	b.WriteString("\n" + keys(w, "Esc", L("назад", "back")) + "\n")
}

// mask скрывает секреты: в TUI их могут увидеть через плечо или на скриншоте.
func mask(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	if len(r) <= 8 {
		return strings.Repeat("•", len(r))
	}
	return string(r[:4]) + "••••" + string(r[len(r)-4:])
}

func keys(w int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, stKey.Render(pairs[i])+" "+stDim.Render(pairs[i+1]))
	}
	// Не влезает — переносим на вторую строку, а не обрезаем.
	var lines []string
	cur := ""
	for _, p := range parts {
		cand := p
		if cur != "" {
			cand = cur + stDim.Render(" · ") + p
		}
		if lipgloss.Width(" "+cand) > w && cur != "" {
			lines = append(lines, " "+cur)
			cur = p
		} else {
			cur = cand
		}
	}
	return strings.Join(append(lines, " "+cur), "\n")
}

func (m *model) footerMsg() string {
	if m.busy != "" {
		return " " + stWarn.Render(spinner[m.frame%len(spinner)]+" "+m.busy+"…")
	}
	if m.msg == "" {
		return ""
	}
	first := strings.SplitN(m.msg, "\n", 2)[0]
	if m.msgErr {
		return " " + stErr.Render(trunc(first, max(m.w, 40)-2))
	}
	return " " + stOK.Render(trunc(first, max(m.w, 40)-2))
}

func colorDelay(r PingResult) string {
	s := fmt.Sprintf("%7s", fmtDelay(r))
	switch {
	case r.Delay < 0:
		return stErr.Render(s)
	case r.Delay < 300*time.Millisecond:
		return stOK.Render(s)
	default:
		return stWarn.Render(s)
	}
}

func fmtRate(bps float64) string { return fmtSize(int64(bps)) + L("/с", "/s") }

func fmtSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf(L("%.1f ГБ", "%.1f GB"), float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf(L("%.1f МБ", "%.1f MB"), float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf(L("%.0f КБ", "%.0f KB"), float64(n)/(1<<10))
	}
	return fmt.Sprintf(L("%d Б", "%d B"), n)
}

func trunc(s string, n int) string {
	if n <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
