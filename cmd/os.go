package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/defectdojo/godojo/distros"
	c "github.com/mtesauro/commandeer"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Supported OSes
type targetOS struct {
	id      string
	os      string
	distro  string
	release string
}

func checkOS(d *DDConfig) targetOS {
	// Check install OS
	d.sectionMsg("Determining OS for installation")

	// TODO: write OS determination code for OS X
	// TODO: test OS detection on Alpine Linux docker
	target := targetOS{}
	determineOS(d, &target)

	// Use Caser to correctly do the title case for Enlish (golang.org/x/text/cases)
	c := cases.Title(language.English)
	d.statusMsg(fmt.Sprintf("OS was determined to be %+v, %+v", c.String(target.os), c.String(target.id)))
	d.statusMsg("DefectDojo installation on this OS is supported, continuing")

	return target
}

func determineOS(d *DDConfig, tOS *targetOS) {
	// Determine OS first
	tOS.os = runtime.GOOS
	d.traceMsg(fmt.Sprintf("Determining OS based on GOOS: %+v", tOS.os))

	switch tOS.os {
	case "linux":
		d.traceMsg("OS determined to be Linux")
		determineLinux(d, tOS)
	case "darwin":
		d.traceMsg("OS determined to be Darwin/OS X")
		fmt.Println("OS X/Darwin")
		d.errorMsg("OS X is not YET a supported installation platform")
		os.Exit(1)
	case "windows":
		d.traceMsg("OS determined to be Windows")
		d.errorMsg("Windows is not a supported installation platform")
		os.Exit(1)
	}
}

func determineLinux(d *DDConfig, tOS *targetOS) {
	// Determine the Linux Distro the installer is running on
	// Based on Based on https://unix.stackexchange.com/questions/6345/how-can-i-get-distribution-name-and-version-number-in-a-simple-shell-script
	d.traceMsg("Determining what Linux distro is the target OS")

	// freedesktop.org and systemd
	_, err := os.Stat("/etc/os-release")
	if err == nil {
		// That file exists
		d.traceMsg("Determining Linux distro from /etc/os-release")
		tOS.distro, tOS.release, tOS.id = parseOSRelease(d, "/etc/os-release")
		if strings.Contains(strings.ToLower(tOS.distro), "rocky") {
			d.traceMsg("Linux distro is Rocky Linux")
			d.traceMsg("Treating Rocky Linux as RHEL for remainder of the install")
			d.statusMsg("Identified Rocky Linux which is compatible with RHEL.")
			d.statusMsg("Using RHEL install method going forward...")
			tOS.distro = "rhel"
			tOS.release = onlyMajorVer(tOS.release)
			tOS.id = tOS.distro + ":" + tOS.release
			return
		}
		if strings.Contains(strings.ToLower(tOS.distro), "rhel") {
			d.traceMsg("Linux distro is RHEL")
			tOS.distro = "rhel"
			tOS.release = onlyMajorVer(tOS.release)
			tOS.id = tOS.distro + ":" + tOS.release
			return
		}
		return
	}

	// lsb_release command is present
	lsbCmd, err := exec.LookPath("lsb_release")
	if err == nil {
		// The command was found
		d.traceMsg("Determining Linux distro from lsb_release command")
		tOS.distro, tOS.release, tOS.id = parseLsbCmd(d, lsbCmd)
		return
	}

	// /etc/lsb-release is present
	_, err = os.Stat("/etc/lsb-release")
	if err == nil {
		// The file was found
		d.traceMsg("Determining Linux distro from /etc/lsb-release")
		tOS.distro, tOS.release, tOS.id = parseEtcLsb(d, "/etc/lsb-release")
		return
	}

	// /etc/issue is present
	_, err = os.Stat("/etc/issue")
	if err == nil {
		// The file was found
		d.traceMsg("Determining Linux distro from /etc/issue")
		tOS.distro, tOS.release, tOS.id = parseEtcIss(d, "/etc/issue")
		return
	}

	// /etc/debian_version is present
	_, err = os.Stat("/etc/debian_version")
	if err == nil {
		// The file was found
		d.traceMsg("Determining Linux distro from /etc/debian_version")
		tOS.distro, tOS.release, tOS.id = parseEtcDeb(d, "/etc/debian_version")
		return
	}

	// Older SUSE Linux installation
	_, err = os.Stat("/etc/SuSe-release")
	if err == nil {
		// Distro is too old, not supported
		d.traceMsg("Older SuSe Linux distro isn't supported by this installer")
		d.errorMsg("Older versions of SuSe Linux are not suppported, quitting")
		os.Exit(1)
	}

	// RHEL's way of doing this
	_, err = os.Stat("/etc/redhat-release")
	if err == nil {
		// Distro is too old, not supported
		d.traceMsg("Older RedHat Linux distros aren't supported by this installer")
		d.errorMsg("Older versions of Redhat Linux are not suppported, quitting")
		os.Exit(1)
	}

	d.traceMsg("Unable to determine the linux distro, assuming unsupported.")
	d.errorMsg("Unable to determine the Linux install target, quitting")
	os.Exit(1)
}

func parseOSRelease(d *DDConfig, f string) (string, string, string) {
	// Setup a map of what we need to what /etc/os-release uses
	fields := map[string]string{
		"distro":  "ID",
		"release": "VERSION_ID",
	}
	linMap := parseFile(d, f, "=", fields)

	return linMap["distro"], linMap["release"], linMap["distro"] + ":" + linMap["release"]

}

func onlyMajorVer(v string) string {
	major, _, found := strings.Cut(v, ".")
	if found {
		return major
	}

	return "Bad Version Number"
}

func parseLsbCmd(d *DDConfig, cmd string) (string, string, string) {
	// Setup map to hold parsed values
	vals := make(map[string]string)

	// Execute the lsb_release command with -a (all) and parse the output
	runCmd := exec.Command(cmd, "-a")

	// Run command and gather its output
	cmdOut, err := runCmd.CombinedOutput()
	if err != nil {
		d.errorMsg(fmt.Sprintf("Failed to run OS command, error was: %+v", err))
		os.Exit(1)
	}

	// Parse command output for the strings we need
	lines := bytes.Split(cmdOut, []byte("\n"))
	for _, line := range lines {
		l := string(line)

		// Look for the distro
		if strings.HasPrefix(l, "Distributor ID") {
			dis := strings.SplitN(l, ":", 2)
			vals["distro"] = strings.ToLower(strings.Trim(dis[1], "\n\t\" "))
		}

		// Look for the release
		if strings.HasPrefix(l, "Release") {
			rel := strings.SplitN(l, ":", 2)
			vals["release"] = strings.ToLower(strings.Trim(rel[1], "\n\t\" "))
		}
	}

	if _, ok := vals["distro"]; !ok {
		// The distro key hasn't been set above
		d.errorMsg("Unable to determine distro from lsb_release command, quitting.")
		os.Exit(1)
	}
	if _, ok := vals["release"]; !ok {
		// The distro key hasn't been set above
		d.errorMsg("Unable to determine release from lsb_release command, quitting.")
		os.Exit(1)
	}

	return vals["distro"], vals["release"], vals["distro"] + ":" + vals["release"]
}

func parseEtcLsb(d *DDConfig, f string) (string, string, string) {
	// Setup a map of what we need to what /etc/lsb-release uses
	fields := map[string]string{
		"distro":  "DISTRIB_ID",
		"release": "DISTRIB_RELEASE",
	}
	linMap := parseFile(d, f, "=", fields)

	return linMap["distro"], linMap["release"], linMap["distro"] + ":" + linMap["release"]
}

func parseEtcIss(d *DDConfig, f string) (string, string, string) {
	// Setup return map
	vals := make(map[string]string)

	// Open the file for parsing
	file, err := os.Open(f)
	if err != nil {
		d.errorMsg(fmt.Sprintf("Unable to open file: %+v\nError was: %v", f, err))
		os.Exit(1)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			d.traceMsg(fmt.Sprintf("Erro closing file\nError was: %v", err))
			os.Exit(1)
		}
	}()

	// Read the file in, pull off the first line and split it
	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil {
		d.errorMsg(fmt.Sprintf("Unable to read file: %+v\nError was: %v", f, err))
		os.Exit(1)
	}
	fields := strings.Split(line, " ")
	vals["distro"] = strings.ToLower(fields[0])
	vals["release"] = fields[1]

	// Correct for Ubuntu 'minor' releases aka 18.04.2
	if vals["distro"] == "ubuntu" {
		tmp := strings.Split(vals["release"], ".")
		vals["release"] = tmp[0] + "." + tmp[1]
	}

	return vals["distro"], vals["release"], vals["distro"] + ":" + vals["release"]
}

func parseEtcDeb(d *DDConfig, f string) (string, string, string) {
	// Setup map to hold parsed values
	vals := make(map[string]string)
	vals["distro"] = "debian"

	// Open the file for parsing
	file, err := os.Open(f)
	if err != nil {
		d.errorMsg(fmt.Sprintf("Unable to open file: %+v\nError was: %v", f, err))
		os.Exit(1)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			d.errorMsg(fmt.Sprintf("Unable to close file\nError was: %v", err))
			os.Exit(1)
		}
	}()

	// Read the file in, pull off the first line
	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil {
		d.errorMsg(fmt.Sprintf("Unable to read file: %+v\nError was: %v", f, err))
		os.Exit(1)
	}
	// TODO: Test this with a Debian docker
	vals["release"] = strings.ToLower(strings.Trim(line, "\n\t "))

	return vals["distro"], vals["release"], vals["distro"] + ":" + vals["release"]
}

func parseFile(d *DDConfig, f string, sep string, flds map[string]string) map[string]string {
	// Setup return map
	vals := make(map[string]string)

	// Open the file for parsing
	file, err := os.Open(f)
	if err != nil {
		d.errorMsg(fmt.Sprintf("Unable to open file: %+v\nError was: %v", f, err))
		os.Exit(1)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			d.errorMsg(fmt.Sprintf("Unable to close file\nError was: %v", err))
			os.Exit(1)
		}
	}()

	// Read the file one line at a time till done
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		// Look for the distro
		if strings.HasPrefix(line, flds["distro"]+sep) {
			dis := strings.SplitN(line, sep, 2)
			vals["distro"] = strings.ToLower(strings.Trim(dis[1], "\n\""))
		}

		// Look for the release
		if strings.HasPrefix(line, flds["release"]+sep) {
			rel := strings.SplitN(line, sep, 2)
			vals["release"] = strings.ToLower(strings.Trim(rel[1], "\n\""))
		}
	}

	return vals
}

// prepOSForDojo takes a pointer to a DDConfig struct and a string representing
// the id for the target OS and installs the necessary OS software required by
// DefectDojo
func prepOSForDojo(d *DDConfig, t *targetOS) {
	// Gather OS commands to bootstrap the install
	d.sectionMsg("Installing OS packages needed for DefectDojo")

	// Create a new installerprep command package
	cInstallerPrep := c.NewPkg("installerprep")

	// Get commands for the right distro
	switch {
	case t.distro == "ubuntu":
		//case "ubuntu":
		d.traceMsg("Searching for commands to prep for the installer on Ubuntu")
		err := distros.GetUbuntu(cInstallerPrep, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to bootstrap target OS %s\n", t.id)
			os.Exit(1)
		}
	case strings.ToLower(t.distro) == "rhel":
		d.traceMsg("Searching for commands for bootstrapping RHEL")
		err := distros.GetRHEL(cInstallerPrep, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to bootstrap target OS %s\n", t.id)
			os.Exit(1)
		}
	default:
		d.traceMsg(fmt.Sprintf("Distro identified (%s) is not supported", t.id))
		fmt.Printf("Distro identified by godojo (%s) is not supported, exiting...\n", t.id)
		os.Exit(1)
	}

	// Install the OS packages
	d.spin = spinner.New(spinner.CharSets[34], 100*time.Millisecond)
	d.spin.Prefix = "Installing OS packages..."
	d.spin.Start()
	// Run the installer prep commands for the target OS
	d.traceMsg(fmt.Sprintf("Getting commands to bootstrap %s", t.id))
	tCmds, err := distros.CmdsForTarget(cInstallerPrep, t.id)
	if err != nil {
		fmt.Printf("Error getting commands to bootstrap target OS %s\n", t.id)
		os.Exit(1)
	}

	// Inject values from config into commands
	d.injectConfigVals(tCmds)

	for i := range tCmds {
		sendCmd(d,
			d.cmdLogger,
			tCmds[i].Cmd,
			tCmds[i].Errmsg,
			tCmds[i].Hard)
	}
	d.spin.Stop()
	d.statusMsg("Installing OS packages complete")
}

// prepDjango(d, &osTarget)
func prepDjango(d *DDConfig, t *targetOS) {
	// Prep OS for Django framework (user, virtualenv, chownership)
	d.sectionMsg("Preparing the OS for DefectDojo installation")

	// Create new prep Django command package
	cPrepDjango := c.NewPkg("prepdjango")

	// Get commands for the right distro
	switch {
	case t.distro == "ubuntu":
		d.traceMsg("Searching for commands to prep Django on Ubuntu")
		err := distros.GetUbuntu(cPrepDjango, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to prep Django target OS %s\n", t.id)
			os.Exit(1)
		}
	case t.distro == "rhel":
		d.traceMsg("Searching for commands to prep Django on RHEL")
		err := distros.GetRHEL(cPrepDjango, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to prep Django target OS %s\n", t.id)
			os.Exit(1)
		}
	default:
		d.traceMsg(fmt.Sprintf("Distro identified (%s) is not supported", t.id))
		fmt.Printf("Distro identified by godojo (%s) is not supported, exiting...\n", t.id)
		os.Exit(1)
	}

	// Start the spinner
	d.spin = spinner.New(spinner.CharSets[34], 100*time.Millisecond)
	d.spin.Prefix = "Preparing the OS for DefectDojo..."
	d.spin.Start()
	// Run the prep Django commands for the target OS
	d.traceMsg(fmt.Sprintf("Getting commands to prep Django on %s", t.id))
	tCmds, err := distros.CmdsForTarget(cPrepDjango, t.id)
	if err != nil {
		fmt.Printf("Error getting commands to bootstrap target OS %s\n", t.id)
		os.Exit(1)
	}

	// Inject values from config into commands
	d.injectConfigVals(tCmds)

	for i := range tCmds {
		sendCmd(d,
			d.cmdLogger,
			tCmds[i].Cmd,
			tCmds[i].Errmsg,
			tCmds[i].Hard)
	}
	d.spin.Stop()
	d.statusMsg("Preparing the OS complete")
}

// createSettings
func createSettings(d *DDConfig, t *targetOS) {
	// Create settings.py for DefectDojo
	// TODO: Update this to use local_settings.py
	d.sectionMsg("Creating settings.py for DefectDojo")

	// Write out the settings file
	// TODO: Update this to local_settings.py
	createSettingsPy(d)

	// Create new create settings command package
	cCreateSettings := c.NewPkg("createsettings")

	// Get commands for the right distro
	switch {
	case t.distro == "ubuntu":
		d.traceMsg("Searching for commands to create settings on Ubuntu")
		err := distros.GetUbuntu(cCreateSettings, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to create settings target OS %s\n", t.id)
			os.Exit(1)
		}
	case t.distro == "rhel":
		d.traceMsg("Searching for commands to create settings on RHEL")
		err := distros.GetRHEL(cCreateSettings, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to create settings target OS %s\n", t.id)
			os.Exit(1)
		}
	default:
		d.traceMsg(fmt.Sprintf("Distro identified (%s) is not supported", t.id))
		fmt.Printf("Distro identified by godojo (%s) is not supported, exiting...\n", t.id)
		os.Exit(1)
	}

	// Start the spinner
	d.spin = spinner.New(spinner.CharSets[34], 100*time.Millisecond)
	d.spin.Prefix = "Creating settings.py for DefectDojo..."
	d.spin.Start()
	// Run the create settings commands for the target OS
	d.traceMsg(fmt.Sprintf("Getting commands to create settings on %s", t.id))
	tCmds, err := distros.CmdsForTarget(cCreateSettings, t.id)
	if err != nil {
		fmt.Printf("Error getting commands to bootstrap target OS %s\n", t.id)
		os.Exit(1)
	}

	// Inject values from config into commands
	d.injectConfigVals(tCmds)

	for i := range tCmds {
		sendCmd(d,
			d.cmdLogger,
			tCmds[i].Cmd,
			tCmds[i].Errmsg,
			tCmds[i].Hard)
	}
	d.spin.Stop()
	d.statusMsg("Creating settings.py for DefectDojo complete")

}

// createSettingsPy
func createSettingsPy(d *DDConfig) {
	// Setup the env.prod file used by settings.py

	// Create the database URL for the env file - https://github.com/kennethreitz/dj-database-url
	dbURL := ""
	switch d.conf.Install.DB.Engine {
	case "SQLite":
		// sqlite:///PATH
		dbURL = "sqlite:///defectdojo.db"
	case "MySQL":
		// mysql://USER:PASSWORD@HOST:PORT/NAME
		dbURL = "mysql://" + d.conf.Install.DB.User + ":" + d.conf.Install.DB.Pass + "@" + d.conf.Install.DB.Host + ":" +
			strconv.Itoa(d.conf.Install.DB.Port) + "/" + d.conf.Install.DB.Name
	case "PostgreSQL":
		// postgres://USER:PASSWORD@HOST:PORT/NAME
		dbURL = "postgres://" + d.conf.Install.DB.User + ":" + d.conf.Install.DB.Pass + "@" + d.conf.Install.DB.Host + ":" +
			strconv.Itoa(d.conf.Install.DB.Port) + "/" + d.conf.Install.DB.Name
	}

	// Setup env file for production
	genAndWriteEnv(d, dbURL)

}

// setupDefectDojo
func setupDefectDojo(d *DDConfig, t *targetOS) {
	d.sectionMsg("Setting up Django for DefectDojo")

	// Create new setup DefectDojo command package
	cSetupDojo := c.NewPkg("setupdojo")

	// Get commands for the right distro
	switch {
	case t.distro == "ubuntu":
		d.traceMsg("Searching for commands to setup DefectDojo on Ubuntu")
		err := distros.GetUbuntu(cSetupDojo, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to setup DefectDojo on target OS %s\n", t.id)
			os.Exit(1)
		}
	case t.distro == "rhel":
		d.traceMsg("Searching for commands to setup DefectDojo on RHEL")
		err := distros.GetRHEL(cSetupDojo, t.id)
		if err != nil {
			fmt.Printf("Error searching for commands to setup DefectDojo on target OS %s\n", t.id)
			os.Exit(1)
		}
	default:
		d.traceMsg(fmt.Sprintf("Distro identified (%s) is not supported", t.id))
		fmt.Printf("Distro identified by godojo (%s) is not supported, exiting...\n", t.id)
		os.Exit(1)
	}

	// Start the spinner
	d.spin = spinner.New(spinner.CharSets[34], 100*time.Millisecond)
	d.spin.Prefix = "Setting up Django for DefectDojo..."
	d.spin.Start()
	// Run the setup DefectDojo commands for the target OS
	d.traceMsg(fmt.Sprintf("Getting commands to setup DefectDojo on %s", t.id))
	tCmds, err := distros.CmdsForTarget(cSetupDojo, t.id)
	if err != nil {
		fmt.Printf("Error getting commands to setup DefectDojo on target OS %s\n", t.id)
		os.Exit(1)
	}

	// Inject values from config into commands
	d.injectConfigVals(tCmds)

	for i := range tCmds {
		sendCmd(d,
			d.cmdLogger,
			tCmds[i].Cmd,
			tCmds[i].Errmsg,
			tCmds[i].Hard)
	}
	d.spin.Stop()
	d.statusMsg("Setting up Django complete")
}
