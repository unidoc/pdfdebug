package linux_packaging_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageInstallsBothBinariesUnderUsrBin(t *testing.T) {
	gui := nfpmEntry(t, installedGUI)
	if !strings.Contains(gui, `"./bin/unidoc-pdf-debugger"`) {
		t.Errorf("%s entry does not install bin/unidoc-pdf-debugger: %s", installedGUI, gui)
	}
	cli := nfpmEntry(t, installedCLI)
	if !strings.Contains(cli, `"./bin/pdfdebug"`) {
		t.Errorf("%s entry does not install bin/pdfdebug: %s", installedCLI, cli)
	}
	for name, e := range map[string]string{installedGUI: gui, installedCLI: cli} {
		if !strings.Contains(e, "mode: 0755") {
			t.Errorf("%s is not installed executable", name)
		}
	}
}

func TestPackageVersionAndArchComeFromTheEnvironment(t *testing.T) {
	src := readRepoFile(t, nfpmConfig)
	for _, want := range []string{`version: "${VERSION}"`, `arch: "${GOARCH}"`, `name: "unidoc-pdf-debugger"`, `license: "Apache-2.0"`} {
		if !strings.Contains(src, want) {
			t.Errorf("%s lacks %s", nfpmConfig, want)
		}
	}
}

func TestAppArmorProfileShipsOnlyInTheDebAsAConffile(t *testing.T) {
	e := nfpmEntry(t, installedRules)
	if !strings.Contains(e, `"./`+appArmorFile+`"`) {
		t.Errorf("%s entry does not install %s", installedRules, appArmorFile)
	}
	if !strings.Contains(e, "packager: deb") {
		t.Errorf("%s must be limited to the .deb", installedRules)
	}
	if !strings.Contains(e, "type: config") {
		t.Errorf("%s must be a config file so dpkg keeps local edits", installedRules)
	}
}

func TestAppArmorProfileAllowsUserNamespacesForTheInstalledBinary(t *testing.T) {
	src := readRepoFile(t, appArmorFile)
	for _, want := range []string{
		"abi <abi/4.0>,",
		"profile unidoc-pdf-debugger " + installedGUI + " flags=(unconfined) {",
		"  userns,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("%s lacks %q", appArmorFile, want)
		}
	}
}

func TestPackageScriptsAreWired(t *testing.T) {
	src := readRepoFile(t, nfpmConfig)
	for _, want := range []string{
		`postinstall: "./` + postinstall + `"`,
		`postremove: "./` + postremove + `"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("%s lacks %s", nfpmConfig, want)
		}
	}
}

func TestPostinstallLoadsTheProfileWhenAppArmorIsActive(t *testing.T) {
	src := readRepoFile(t, postinstall)
	for _, want := range []string{
		"profile=" + installedRules,
		"command -v apparmor_parser",
		"/sys/kernel/security/apparmor",
		`apparmor_parser -r "$profile"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("%s lacks %q", postinstall, want)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(src), "exit 0") {
		t.Errorf("%s must end with exit 0 so a missing tool never fails the install", postinstall)
	}
}

func TestPostremoveUnloadsTheProfileOnlyOnRemove(t *testing.T) {
	src := readRepoFile(t, postremove)
	i := strings.Index(src, "remove)")
	j := strings.Index(src, `apparmor_parser -R "$profile"`)
	if i < 0 || j < i {
		t.Fatalf("%s must unload the profile inside its remove) case", postremove)
	}
	if strings.Contains(src, "upgrade)") || strings.Contains(src, "purge)") {
		t.Errorf("%s must not unload the profile on upgrade or purge", postremove)
	}
}

func TestDesktopEntryRunsTheInstalledBinary(t *testing.T) {
	src := readRepoFile(t, desktopEntry)
	for _, want := range []string{"Exec=unidoc-pdf-debugger %f", "Icon=unidoc-pdf-debugger", "MimeType=application/pdf;"} {
		if !strings.Contains(src, want) {
			t.Errorf("%s lacks %s", desktopEntry, want)
		}
	}
	nfpmEntry(t, "/usr/share/pixmaps/unidoc-pdf-debugger.png")
	nfpmEntry(t, "/usr/share/applications/unidoc-pdf-debugger.desktop")
}

func TestPackagingFilesAreTracked(t *testing.T) {
	out, err := exec.Command("git", "-C", projectRoot(t), "ls-files", "build/linux").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	tracked := string(out)
	for _, f := range []string{nfpmConfig, appArmorFile, postinstall, postremove, desktopEntry} {
		if !strings.Contains(tracked, f+"\n") {
			t.Errorf("%s is not tracked by git", f)
		}
	}
}
