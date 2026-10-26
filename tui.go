package main

import (
	"fmt"
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
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type (
	tickMsg   struct{}
	statusMsg struct{ running bool }
	opMsg     struct {
		text string
		err  error
		st   *State // новое состояние, если операция его меняла
	}
	pingMsg struct {
		res []PingResult
		err error
	}
)

type model struct {
	st       *State
	cursor   int
	offset   int
	pings    []PingResult
	running  bool
	busy     string
	frame    int
	msg      string
	msgErr   bool
	input    bool // ввод URL подписки
	inputBuf string
	w, h     int
}

func runTUI(st *State) error {
	m := &model{st: st, w: 80, h: 24}
	if st.Selected >= 0 {
		m.cursor = st.Selected
	}
	m.running, _ = isRunning()
	if len(st.All()) == 0 {
		m.input = true
		m.inputBuf = st.SubURL
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(*model); ok && fm.running {
		fmt.Println("VPN продолжает работать в фоне. Выключить: vpn off")
	}
	return nil
}

func (m *model) Init() tea.Cmd { return tea.Batch(tick(), checkStatus()) }

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func checkStatus() tea.Cmd {
	return func() tea.Msg { r, _ := isRunning(); return statusMsg{r} }
}

func (m *model) start(label string, cmd tea.Cmd) tea.Cmd {
	m.busy, m.msg = label, ""
	return cmd
}

func (m *model) setMsg(text string, isErr bool) { m.msg, m.msgErr = text, isErr }

// copyState — фоновые операции работают с копией, чтобы не гоняться с отрисовкой.
func (m *model) copyState() *State { cp := *m.st; return &cp }

func opOn(st *State) tea.Cmd {
	return func() tea.Msg {
		if err := vpnOn(st, func(string) {}); err != nil {
			return opMsg{err: err}
		}
		p, _ := st.Current()
		return opMsg{text: "Включено: " + p.Name}
	}
}

func opOff(st *State) tea.Cmd {
	return func() tea.Msg {
		if err := vpnOff(st); err != nil {
			return opMsg{text: "Выключено (предупреждение: " + err.Error() + ")"}
		}
		return opMsg{text: "Выключено"}
	}
}

func opUpdate(st *State) tea.Cmd {
	return func() tea.Msg {
		n, skipped, err := updateSub(st)
		if err != nil {
			return opMsg{err: err}
		}
		t := fmt.Sprintf("Подписка обновлена: %d серверов", n)
		if len(skipped) > 0 {
			t += fmt.Sprintf(", пропущено %d", len(skipped))
		}
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
		if m.frame%20 == 0 && m.busy == "" { // раз в 2 с проверяем, жив ли xray
			cmds = append(cmds, checkStatus())
		}
		return m, tea.Batch(cmds...)
	case statusMsg:
		m.running = msg.running
	case opMsg:
		m.busy = ""
		if msg.st != nil {
			m.st = msg.st
			m.pings = nil
			m.st.Save()
			if m.cursor >= len(m.st.All()) {
				m.cursor = 0
			}
		}
		if msg.err != nil {
			m.setMsg(msg.err.Error(), true)
		} else {
			m.setMsg(msg.text, false)
		}
		return m, checkStatus()
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
			m.setMsg(fmt.Sprintf("Отвечают %d из %d", alive, len(msg.res)), false)
		}
	case tea.KeyMsg:
		if m.input {
			return m.updateInput(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m *model) updateInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		if len(m.st.All()) == 0 {
			return m, tea.Quit
		}
		m.input = false
	case tea.KeyEnter:
		u := strings.TrimSpace(m.inputBuf)
		if !strings.HasPrefix(u, "http") {
			m.setMsg("Нужна ссылка вида https://…", true)
			return m, nil
		}
		m.input = false
		m.st.SubURL = u
		m.st.Save()
		return m, m.start("Скачиваю подписку", opUpdate(m.copyState()))
	case tea.KeyBackspace:
		if r := []rune(m.inputBuf); len(r) > 0 {
			m.inputBuf = string(r[:len(r)-1])
		}
	case tea.KeyCtrlU:
		m.inputBuf = ""
	case tea.KeyRunes: // вставка из буфера тоже приходит сюда
		m.inputBuf += string(k.Runes)
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
		return m, m.start("Подключаюсь", opOn(m.copyState()))
	case " ", "o":
		if m.running {
			return m, m.start("Выключаю", opOff(m.copyState()))
		}
		if _, ok := m.st.Current(); !ok && n > 0 {
			m.st.Selected = m.cursor
			m.st.Save()
		}
		return m, m.start("Подключаюсь", opOn(m.copyState()))
	case "p":
		if n > 0 {
			return m, m.start("TCP-пинг", opPing(m.copyState(), false))
		}
	case "P":
		if n > 0 {
			return m, m.start("Реальный пинг через каждый сервер", opPing(m.copyState(), true))
		}
	case "b":
		if m.pings != nil {
			best := -1
			for i, r := range m.pings {
				if r.Delay > 0 && (best < 0 || r.Delay < m.pings[best].Delay) {
					best = i
				}
			}
			if best >= 0 {
				m.cursor = best
				m.setMsg("Самый быстрый — Enter, чтобы подключиться", false)
			}
		} else {
			m.setMsg("Сначала пинг: p или P", true)
		}
	case "u":
		if m.st.SubURL == "" {
			m.input = true
			return m, nil
		}
		return m, m.start("Обновляю подписку", opUpdate(m.copyState()))
	case "s":
		m.input, m.inputBuf = true, m.st.SubURL
	case "m":
		if m.st.Mode == "ru" {
			m.st.Mode = "global"
		} else {
			m.st.Mode = "ru"
		}
		m.st.Save()
		if m.running {
			return m, m.start("Применяю режим", opOn(m.copyState()))
		}
		m.setMsg("Режим: "+m.st.Mode, false)
	}
	return m, nil
}

func (m *model) View() string {
	var b strings.Builder
	w := max(m.w, 40)

	// Статус
	status := stOff.Render("○ ВЫКЛ")
	if m.running {
		status = stOn.Render("● ВКЛ")
	}
	cur := "сервер не выбран"
	if p, ok := m.st.Current(); ok {
		cur = p.Name
	}
	b.WriteString(fmt.Sprintf(" %s  %s  %s\n", status, stTitle.Render(trunc(cur, w-30)), stDim.Render("режим "+m.st.Mode)))
	info := fmt.Sprintf(" SOCKS :%d · HTTP :%d · SSH напрямую", m.st.SocksPort, m.st.HTTPPort)
	if l := subLine(m.st); l != "" {
		info += " · " + l
	}
	b.WriteString(stDim.Render(trunc(info, w)) + "\n\n")

	if m.input {
		b.WriteString(" Вставьте ссылку на подписку от хостера (https://…) и нажмите Enter:\n\n")
		b.WriteString(" > " + trunc(m.inputBuf, w-5) + "█\n\n")
		b.WriteString(stDim.Render(" Ctrl+U — очистить · Esc — отмена") + "\n")
		b.WriteString(m.footerMsg())
		return b.String()
	}

	all := m.st.All()
	rows := max(m.h-7, 3)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if len(all) == 0 {
		b.WriteString(" Серверов нет. s — задать подписку.\n")
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
		row := line + fmt.Sprintf("%-*s", w-lipgloss.Width(line)-9, name)
		if i == m.cursor {
			row = stSel.Render(row)
		}
		b.WriteString(row + " " + delay + "\n")
	}
	if len(all) > rows {
		b.WriteString(stDim.Render(fmt.Sprintf(" … %d–%d из %d", m.offset+1, min(m.offset+rows, len(all)), len(all))) + "\n")
	}
	b.WriteString("\n" + stDim.Render(trunc(" Enter подключить · Space вкл/выкл · p/P пинг · b лучший · u обновить · m режим · s подписка · q выход", w)) + "\n")
	b.WriteString(m.footerMsg())
	return b.String()
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
