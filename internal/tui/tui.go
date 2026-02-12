package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/alfaoz/beammeup/internal/hangar"
	"github.com/alfaoz/beammeup/internal/session"
	"github.com/alfaoz/beammeup/internal/ships"
	"github.com/charmbracelet/huh"
)

type App struct {
	Store     *ships.Store
	HangarSvc *hangar.Service
	Secrets   *session.PasswordCache
	status    map[string]hangar.Status
}

var (
	errExitRequested = errors.New("exit requested")
	errUserCancelled = errors.New("user cancelled")
)

func New(store *ships.Store, svc *hangar.Service, sec *session.PasswordCache) *App {
	return &App{Store: store, HangarSvc: svc, Secrets: sec, status: map[string]hangar.Status{}}
}

func (a *App) Run() error {
	for {
		shipNames, err := a.Store.List()
		if err != nil {
			return err
		}
		if len(shipNames) == 0 {
			if err := a.onboardNoShips(); err != nil {
				if errors.Is(err, errExitRequested) || errors.Is(err, errUserCancelled) {
					return nil
				}
				return err
			}
			continue
		}

		description := "persistent cockpit"
		if lines := a.shipSummaryLines(shipNames); lines != "" {
			description = lines
		}

		choice := ""
		if err := huh.NewSelect[string]().
			Title("beammeup :: main deck").
			Description(description).
			Options(
				huh.NewOption("Select Ship", "select"),
				huh.NewOption("Create Ship", "create"),
				huh.NewOption("Abandon Ship", "abandon"),
				huh.NewOption("Exit", "exit"),
			).
			Value(&choice).
			Run(); err != nil {
			if isUserCancelled(err) {
				return nil
			}
			return err
		}

		switch choice {
		case "select":
			name, err := a.pickShip(shipNames)
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			if name == "" {
				continue
			}
			ship, err := a.Store.Load(name)
			if err != nil {
				a.note("load failed", err.Error())
				continue
			}
			if err := a.shipCockpit(ship); err != nil {
				a.note("error", err.Error())
			}
		case "create":
			ship, err := a.createShipForm(ships.Ship{})
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			if ship.Name == "" {
				continue
			}
			if err := a.ensureHangarCreated(ship, true); err != nil {
				a.note("hangar setup failed", err.Error())
			}
		case "abandon":
			name, err := a.pickShip(shipNames)
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			if name == "" {
				continue
			}
			if a.confirm("abandon ship " + name + "?") {
				if err := a.Store.Delete(name); err != nil {
					a.note("abandon failed", err.Error())
				} else {
					a.Secrets.Forget(name)
					delete(a.status, name)
					a.note("ship abandoned", "local profile deleted")
				}
			}
		case "exit":
			return nil
		}
	}
}

func (a *App) onboardNoShips() error {
	choice := ""
	if err := huh.NewSelect[string]().
		Title("welcome aboard").
		Description("you have no ships yet").
		Options(
			huh.NewOption("Create Ship", "create"),
			huh.NewOption("Exit", "exit"),
		).
		Value(&choice).
		Run(); err != nil {
		if isUserCancelled(err) {
			return errUserCancelled
		}
		return err
	}
	if choice == "exit" {
		return errExitRequested
	}

	ship, err := a.createShipForm(ships.Ship{})
	if err != nil {
		if errors.Is(err, errUserCancelled) {
			return nil
		}
		return err
	}
	if ship.Name == "" {
		return nil
	}
	if err := a.ensureHangarCreated(ship, true); err != nil {
		a.note("hangar setup failed", err.Error())
	}
	return nil
}

func (a *App) shipCockpit(ship ships.Ship) error {
	for {
		status := a.statusBadge(ship.Name)
		choice := ""
		title := fmt.Sprintf("ship cockpit :: %s (%s)", ship.Name, status)
		if err := huh.NewSelect[string]().
			Title(title).
			Options(
				huh.NewOption("Launch", "launch"),
				huh.NewOption("Hangar", "hangar"),
				huh.NewOption("Edit Ship", "edit"),
				huh.NewOption("Forget Session Password", "forget"),
				huh.NewOption("Abandon Ship", "abandon"),
				huh.NewOption("Back to Main Deck", "back"),
			).
			Value(&choice).
			Run(); err != nil {
			if isUserCancelled(err) {
				return nil
			}
			return err
		}

		switch choice {
		case "launch":
			if err := a.launchShip(ship); err != nil {
				a.note("launch failed", err.Error())
			}
		case "hangar":
			if err := a.hangarMenu(ship); err != nil {
				a.note("hangar error", err.Error())
			}
		case "edit":
			updated, err := a.createShipForm(ship)
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			if updated.Name != "" {
				if updated.Name != ship.Name {
					_ = a.Store.Delete(ship.Name)
					if p, ok := a.Secrets.Get(ship.Name); ok {
						a.Secrets.Set(updated.Name, p)
						a.Secrets.Forget(ship.Name)
					}
					delete(a.status, ship.Name)
				}
				ship = updated
			}
		case "forget":
			a.Secrets.Forget(ship.Name)
			a.note("forgotten", "session password removed")
		case "abandon":
			if a.confirm("abandon ship " + ship.Name + "?") {
				if err := a.Store.Delete(ship.Name); err != nil {
					a.note("abandon failed", err.Error())
					continue
				}
				a.Secrets.Forget(ship.Name)
				delete(a.status, ship.Name)
				a.note("ship abandoned", "local profile deleted")
				return nil
			}
		case "back":
			return nil
		}
	}
}

func (a *App) hangarMenu(ship ships.Ship) error {
	for {
		choice := ""
		if err := huh.NewSelect[string]().
			Title("hangar :: "+ship.Name).
			Options(
				huh.NewOption("Show Configuration", "show"),
				huh.NewOption("Configure/Repair", "configure"),
				huh.NewOption("Rotate Credentials", "rotate"),
				huh.NewOption("Destroy Hangar", "destroy"),
				huh.NewOption("Back", "back"),
			).
			Value(&choice).
			Run(); err != nil {
			if isUserCancelled(err) {
				return nil
			}
			return err
		}
		switch choice {
		case "show":
			inv, err := a.inventoryWithPassword(ship)
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			a.status[ship.Name] = inv.HangarStatus
			a.showInventoryCard(ship, inv)
		case "configure", "rotate":
			protocol, port, noFW, err := a.configurePrompt(ship)
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			in := hangar.ActionInput{
				Mode:              "apply",
				Protocol:          protocol,
				ProxyPort:         port,
				NoFirewallChange:  noFW,
				RotateCredentials: choice == "rotate",
			}
			res, err := a.execWithPassword(ship, in)
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			a.showResultCard(res)
			if inv, err := a.inventoryWithPassword(ship); err == nil {
				a.status[ship.Name] = inv.HangarStatus
			}
		case "destroy":
			if !a.confirm("destroy hangar on " + ship.Host + "?") {
				continue
			}
			confirmText := ""
			if err := huh.NewInput().Title("Type DESTROY to confirm").Value(&confirmText).Run(); err != nil {
				if isUserCancelled(err) {
					continue
				}
				return err
			}
			if strings.TrimSpace(confirmText) != "DESTROY" {
				a.note("cancelled", "destroy confirmation did not match")
				continue
			}
			res, err := a.execWithPassword(ship, hangar.ActionInput{Mode: "destroy"})
			if err != nil {
				if errors.Is(err, errUserCancelled) {
					continue
				}
				return err
			}
			a.status[ship.Name] = hangar.StatusMissing
			a.note("destroy hangar complete", fallback(res.Note, "remote configuration removed"))
			if a.confirm("abandon ship too?") {
				if err := a.Store.Delete(ship.Name); err != nil {
					return err
				}
				a.Secrets.Forget(ship.Name)
				delete(a.status, ship.Name)
				a.note("ship abandoned", "local .ship deleted")
				return nil
			}
		case "back":
			return nil
		}
	}
}

func (a *App) launchShip(ship ships.Ship) error {
	inv, err := a.inventoryWithPassword(ship)
	if err != nil {
		if errors.Is(err, errUserCancelled) {
			return nil
		}
		return err
	}
	a.status[ship.Name] = inv.HangarStatus

	switch inv.HangarStatus {
	case hangar.StatusMissing:
		if !a.confirm("no hangar found. create one now?") {
			return nil
		}
		return a.ensureHangarCreated(ship, false)
	case hangar.StatusDrift:
		choice := ""
		if err := huh.NewSelect[string]().
			Title("hangar drift detected").
			Options(huh.NewOption("Repair", "repair"), huh.NewOption("Recreate", "recreate"), huh.NewOption("Back", "back")).
			Value(&choice).Run(); err != nil {
			if isUserCancelled(err) {
				return nil
			}
			return err
		}
		if choice == "back" {
			return nil
		}
		if choice == "recreate" {
			if _, err := a.execWithPassword(ship, hangar.ActionInput{Mode: "destroy"}); err != nil {
				if errors.Is(err, errUserCancelled) {
					return nil
				}
				return err
			}
		}
		return a.ensureHangarCreated(ship, false)
	default:
		a.showInventoryCard(ship, inv)
		return nil
	}
}

func (a *App) ensureHangarCreated(ship ships.Ship, forcePrompt bool) error {
	if forcePrompt || a.confirm("create hangar now?") {
		protocol := ship.Protocol
		if protocol == "" {
			protocol = "http"
		}
		port := ship.ProxyPort
		if port == 0 {
			if protocol == "socks5" {
				port = 1080
			} else {
				port = 18181
			}
		}
		res, err := a.execWithPassword(ship, hangar.ActionInput{
			Mode:             "apply",
			Protocol:         protocol,
			ProxyPort:        port,
			NoFirewallChange: ship.NoFirewallChange,
		})
		if err != nil {
			return err
		}
		a.status[ship.Name] = hangar.StatusOnline
		a.showResultCard(res)
	}
	return nil
}

func (a *App) configurePrompt(ship ships.Ship) (protocol string, port int, noFW bool, err error) {
	protocol = fallback(ship.Protocol, "http")
	portStr := strconv.Itoa(ship.ProxyPort)
	if ship.ProxyPort == 0 {
		if protocol == "socks5" {
			portStr = "1080"
		} else {
			portStr = "18181"
		}
	}
	noFW = ship.NoFirewallChange

	group := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Protocol").
			Options(huh.NewOption("HTTP", "http"), huh.NewOption("SOCKS5", "socks5")).
			Value(&protocol),
		huh.NewInput().Title("Proxy port").Value(&portStr),
		huh.NewConfirm().Title("Skip firewall changes?").Value(&noFW),
	)
	if err := huh.NewForm(group).Run(); err != nil {
		if isUserCancelled(err) {
			return "", 0, false, errUserCancelled
		}
		return "", 0, false, err
	}
	port, err = strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 {
		return "", 0, false, fmt.Errorf("invalid proxy port: %s", portStr)
	}
	ship.Protocol = protocol
	ship.ProxyPort = port
	ship.NoFirewallChange = noFW
	if _, saveErr := a.Store.Save(ship); saveErr != nil {
		return "", 0, false, saveErr
	}
	return protocol, port, noFW, nil
}

func (a *App) createShipForm(existing ships.Ship) (ships.Ship, error) {
	ship := existing
	name := ship.Name
	host := ship.Host
	sshPort := strconv.Itoa(nonZero(ship.SSHPort, 22))
	sshUser := fallback(ship.SSHUser, "root")
	protocol := fallback(ship.Protocol, "http")
	proxyPort := strconv.Itoa(nonZero(ship.ProxyPort, defaultProxy(protocol)))
	noFW := ship.NoFirewallChange

	group := huh.NewGroup(
		huh.NewInput().Title("Ship name").Value(&name),
		huh.NewInput().Title("Target VPS host/IP").Value(&host),
		huh.NewInput().Title("SSH port").Value(&sshPort),
		huh.NewInput().Title("SSH user").Value(&sshUser),
		huh.NewSelect[string]().
			Title("Default protocol").
			Options(huh.NewOption("HTTP", "http"), huh.NewOption("SOCKS5", "socks5")).
			Value(&protocol),
		huh.NewInput().Title("Default proxy port").Value(&proxyPort),
		huh.NewConfirm().Title("Skip firewall changes by default?").Value(&noFW),
	)

	if err := huh.NewForm(group).Run(); err != nil {
		if isUserCancelled(err) {
			return ships.Ship{}, errUserCancelled
		}
		return ships.Ship{}, err
	}

	name = ships.SanitizeName(name)
	if name == "" {
		return ships.Ship{}, fmt.Errorf("ship name is required")
	}
	port, err := strconv.Atoi(strings.TrimSpace(sshPort))
	if err != nil || port <= 0 {
		return ships.Ship{}, fmt.Errorf("invalid ssh port")
	}
	proxy, err := strconv.Atoi(strings.TrimSpace(proxyPort))
	if err != nil || proxy <= 0 {
		return ships.Ship{}, fmt.Errorf("invalid proxy port")
	}

	ship = ships.Ship{
		Name:             name,
		Host:             strings.TrimSpace(host),
		SSHPort:          port,
		SSHUser:          strings.TrimSpace(sshUser),
		Protocol:         protocol,
		ProxyPort:        proxy,
		NoFirewallChange: noFW,
	}
	return a.Store.Save(ship)
}

func (a *App) pickShip(shipNames []string) (string, error) {
	options := make([]huh.Option[string], 0, len(shipNames)+1)
	for _, s := range shipNames {
		badge := a.statusBadge(s)
		options = append(options, huh.NewOption(fmt.Sprintf("%s  [%s]", s, badge), s))
	}
	options = append(options, huh.NewOption("Back", ""))
	val := ""
	err := huh.NewSelect[string]().Title("Select ship").Options(options...).Value(&val).Run()
	if isUserCancelled(err) {
		return "", errUserCancelled
	}
	return val, err
}

func (a *App) statusBadge(shipName string) string {
	if st, ok := a.status[shipName]; ok {
		return string(st)
	}
	return "unknown"
}

func (a *App) inventoryWithPassword(ship ships.Ship) (hangar.Inventory, error) {
	pwd, err := a.passwordForShip(ship)
	if err != nil {
		return hangar.Inventory{}, err
	}
	inv, err := a.HangarSvc.Inventory(ship, pwd)
	if err != nil {
		return hangar.Inventory{}, err
	}
	a.status[ship.Name] = inv.HangarStatus
	return inv, nil
}

func (a *App) execWithPassword(ship ships.Ship, in hangar.ActionInput) (hangar.ActionResult, error) {
	pwd, err := a.passwordForShip(ship)
	if err != nil {
		return hangar.ActionResult{}, err
	}
	return a.HangarSvc.Execute(ship, pwd, in)
}

func (a *App) passwordForShip(ship ships.Ship) (string, error) {
	if p, ok := a.Secrets.Get(ship.Name); ok && strings.TrimSpace(p) != "" {
		return p, nil
	}
	pwd := ""
	if err := huh.NewInput().EchoMode(huh.EchoModePassword).Title(fmt.Sprintf("SSH password for %s@%s", ship.SSHUser, ship.Host)).Value(&pwd).Run(); err != nil {
		if isUserCancelled(err) {
			return "", errUserCancelled
		}
		return "", err
	}
	if strings.TrimSpace(pwd) == "" {
		return "", fmt.Errorf("password required")
	}
	a.Secrets.Set(ship.Name, pwd)
	return pwd, nil
}

func (a *App) showInventoryCard(ship ships.Ship, inv hangar.Inventory) {
	lines := []string{
		fmt.Sprintf("Ship: %s", ship.Name),
		fmt.Sprintf("Host: %s", fallback(inv.PublicIP, ship.Host)),
		fmt.Sprintf("Hangar: %s", inv.HangarStatus),
		"",
	}
	if inv.HTTP.Exists {
		lines = append(lines, fmt.Sprintf("HTTP   active=%v  port=%s  user=%s", inv.HTTP.Active, fallback(inv.HTTP.Port, "-"), fallback(inv.HTTP.User, "-")))
	}
	if inv.Socks5.Exists {
		lines = append(lines, fmt.Sprintf("SOCKS5 active=%v  port=%s  user=%s", inv.Socks5.Active, fallback(inv.Socks5.Port, "-"), fallback(inv.Socks5.User, "-")))
	}
	if !inv.HTTP.Exists && !inv.Socks5.Exists {
		lines = append(lines, "No hangar services configured.")
	}
	if inv.HTTP.Exists && inv.HTTP.Pass != "" {
		lines = append(lines, "", fmt.Sprintf("HTTP quick test: curl -x 'http://%s:%s@%s:%s' https://api.ipify.org", inv.HTTP.User, inv.HTTP.Pass, ship.Host, inv.HTTP.Port))
	}
	if inv.Socks5.Exists && inv.Socks5.Pass != "" {
		lines = append(lines, "", fmt.Sprintf("SOCKS5 quick test: curl -x 'socks5h://%s:%s@%s:%s' https://api.ipify.org", inv.Socks5.User, inv.Socks5.Pass, ship.Host, inv.Socks5.Port))
	}
	a.note("hangar configuration", strings.Join(lines, "\n"))
}

func (a *App) showResultCard(res hangar.ActionResult) {
	if strings.EqualFold(res.Protocol, "DESTROY") {
		a.note("destroy complete", fallback(res.Note, "hangar removed"))
		return
	}
	msg := []string{
		fmt.Sprintf("Action: %s", res.Action),
		fmt.Sprintf("Protocol: %s", res.Protocol),
		fmt.Sprintf("Host: %s", res.Host),
		fmt.Sprintf("Port: %s", res.Port),
		fmt.Sprintf("Username: %s", fallback(res.User, "-")),
		fmt.Sprintf("Password: %s", fallback(res.Pass, "<not retrievable>")),
	}
	if res.FirewallNote != "" {
		msg = append(msg, "", "Firewall: "+res.FirewallNote)
	}
	if res.Note != "" {
		msg = append(msg, "Note: "+res.Note)
	}
	a.note("mission complete", strings.Join(msg, "\n"))
}

func (a *App) confirm(prompt string) bool {
	val := false
	if err := huh.NewConfirm().Title(prompt).Affirmative("Yes").Negative("No").Value(&val).Run(); err != nil {
		return false
	}
	return val
}

func (a *App) note(title, body string) {
	_ = huh.NewNote().Title(title).Description(body).Next(true).Run()
}

func (a *App) shipSummaryLines(shipNames []string) string {
	lines := []string{"select a ship to open cockpit"}
	for _, name := range shipNames {
		lines = append(lines, fmt.Sprintf("%s [%s]", name, a.statusBadge(name)))
	}
	return strings.Join(lines, "\n")
}

func isUserCancelled(err error) bool {
	if err == nil {
		return false
	}
	v := strings.ToLower(err.Error())
	return strings.Contains(v, "interrupt") || strings.Contains(v, "cancel") || strings.Contains(v, "abort")
}

func fallback(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func nonZero(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

func defaultProxy(protocol string) int {
	if protocol == "socks5" {
		return 1080
	}
	return 18181
}
