package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/james4k/rcon"
	"github.com/joho/godotenv"
)

type ServerController struct {
	hasBossBar        bool
	rcon              *rcon.RemoteConsole
	bossBarSelector   string
	broadcastSelector string
	detailSelector    string
	WarningDelay      int
}

func (m *ServerController) Init(remote *rcon.RemoteConsole) {
	m.hasBossBar = false
	m.rcon = remote

	// Set the BossBar selector.
	bossBarSelector, ok := os.LookupEnv("BOSSBAR_SELECTOR")
	if ok {
		m.bossBarSelector = bossBarSelector
	} else {
		m.bossBarSelector = "@a"
	}

	// Set teh Broadcast selector.
	broadcastSelector, ok := os.LookupEnv("BACKUP_BROADCAST_SELECTOR")
	if ok {
		m.broadcastSelector = broadcastSelector
	} else {
		m.broadcastSelector = "@a"
	}

	// Set then backup message selector.
	detailSelector, ok := os.LookupEnv("BACKUP_MESSAGE_SELECTOR")
	if ok {
		m.detailSelector = detailSelector
	} else {
		m.detailSelector = "@a[tag=backups]"
	}

	// Set then backup message selector.
	m.WarningDelay = 5
	warningDelay, ok := os.LookupEnv("WARNING_DELAY")
	if ok {
		value, err := strconv.ParseInt(warningDelay, 10, 0)
		if err != nil {
			log.Println("Failed to parse WARNING_DELAY env var as int.", err)
		} else {
			m.WarningDelay = int(value)
		}
	}
}

func (m *ServerController) Command(command string) (string, error) {
	sendId, err := m.rcon.Write(command)
	// println(command)
	if err != nil {
		return "", err
	}
	response, responseId, err := m.rcon.Read()
	if err != nil {
		return response, err
	}

	if sendId != responseId {
		log.Println("Rcon message ID mismatch.")
	}

	return response, err
}

func (m *ServerController) Tell(selector string, message string, colour string) error {
	if colour == "" {
		colour = "gray"
	}

	response, err := m.Command(`tellraw ` + selector + ` [{"text":"[Backup]","color":"aqua"},{"text":" ` + message + `","color":"` + colour + `"}]`)

	if response != "" {
		return &CommandError{reason: response}
	}
	return err
}

func (m *ServerController) SetProgress(title string, value int) error {
	err := m.Tell(m.detailSelector, title, "")
	if err != nil {
		return err
	}

	if m.hasBossBar {
		_, err = m.Command(`bossbar set backup:active value ` + strconv.Itoa(value))
		if err != nil {
			return err
		}
		_, err = m.Command(`bossbar set backup:active name "Backup: ` + title + `"`)
		if err != nil {
			return err
		}
	}
	return nil
}
func (m *ServerController) ShowBossbar() error {
	// Add the required players to the bossbar.
	response, err := m.Command("bossbar set backup:active players " + m.bossBarSelector)
	if err != nil {
		return err
	}

	// If the boss bar doesn't exist, exit now.
	if strings.HasPrefix("No bossbar exists with the ID", response) {
		return &CommandError{reason: response}
	}

	// Make the bossbar visible, if it's "now visible", set hasBossBar.
	response, err = m.Command(`bossbar set backup:active visible true`)
	if err != nil {
		return err
	}
	m.hasBossBar = true
	return nil
	// return &CommandError{reason: response}
}
func (m *ServerController) HideBossbar() error {
	// Make the bossbar hidden, if it's "now visible", set hasBossBar.
	_, err := m.Command(`bossbar set backup:active visible false`)
	if err != nil {
		return err
	}
	m.hasBossBar = false
	return nil
}

func main() {
	godotenv.Load()

	host := os.Getenv("RCON_HOST")
	pass := os.Getenv("RCON_PASSWORD")

	// Create a connection to the server.
	remote, err := rcon.Dial(host, pass)
	if err != nil {
		log.Fatal("Failed to connect to RCON server at "+host, err)
	}
	defer remote.Close()

	server := &ServerController{}
	server.Init(remote)

	// Tell player that backup starting.
	err = server.Tell(server.broadcastSelector, "Starting a backup shortly.", "")
	if err != nil {
		println("Failed broadcast backup message", err.Error())
	}
	time.Sleep(time.Duration(server.WarningDelay) * time.Second)

	err = server.ShowBossbar()
	if err != nil {
		println("Failed to show the bossbar.", err.Error())
	}
	err = server.SetProgress("Starting Backup", 0)
	if err != nil {
		println("Failed to update the progress.", err)
	}
	// Do the backup.
	Backup(server)
	time.Sleep(2000 * time.Millisecond)
	if server.hasBossBar {
		//log.Println("output: ", cmd.Output())
		server.HideBossbar()
	}
}

func Backup(server *ServerController) {

	// Disable auto-save and force a save.
	server.SetProgress("Saving Worlds", 10)
	server.Command("save-off")
	server.Command("save-all")
	defer server.Command("save-on")

	time.Sleep(1 * time.Second)

	// Run Restic.
	cmd := exec.Command("restic", "backup", "--files-from", ".backuplist")
	server.SetProgress("Running restic", 50)
	log.Println(cmd)
	server.Tell(server.detailSelector, cmd.String(), "")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("restic Failed", err)
		server.Tell(server.detailSelector, "Failed backup. Exit: "+strconv.Itoa(cmd.ProcessState.ExitCode()), "red")
		server.Tell(server.detailSelector, err.Error(), "red")
	}
	output := string(out)
	log.Println(output)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		server.Tell(server.detailSelector, line, "")
	}
	server.SetProgress(lines[len(lines)-1], 100)
}

type CommandError struct {
	reason string
}

func (m *CommandError) Error() string {
	return m.reason
}
