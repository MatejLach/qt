package templater

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/therecipe/qt/internal/binding/parser"
	"github.com/therecipe/qt/internal/utils"
)

const (
	NONE = iota
	MOC
	MINIMAL
	RCC
)

func QmakeCgoTemplate(module, path, target string, mode int, ipkg string) (o string) {

	switch module {
	case "AndroidExtras":
		if target != "android" {
			return
		}
	case "Sailfish":
		if !strings.HasPrefix(target, "sailfish") {
			return
		}
	}

	if path == "" {
		path = utils.GoQtPkgPath(strings.ToLower(module))
	}

	switch target {
	case "darwin", "linux":
		if runtime.GOOS != target {
			return
		}
	case "windows": //can be windows,linux,darwin
	case "android":
		switch module {
		case "DBus", "WebEngine", "Designer":
			return
		}
		if strings.HasSuffix(module, "Extras") && module != "AndroidExtras" {
			return
		}
	case "ios", "ios-simulator":
		switch module {
		case "DBus", "WebEngine", "Designer", "SerialPort", "SerialBus":
			return
		}
		if strings.HasSuffix(module, "Extras") {
			return
		}
	case "sailfish", "sailfish-emulator", "asteroid":
		if !IsWhiteListedSailfishLib(module) {
			return
		}
	case "rpi1", "rpi2", "rpi3":
	default:
		target = runtime.GOOS
	}

	if isAlreadyCached(module, path, target, mode) {
		return
	}
	createProject(module, path, mode)
	createMakefile(module, path, target, mode)
	createCgo(module, path, target, mode, ipkg)

	utils.RemoveAll(filepath.Join(path, "Makefile"))
	utils.RemoveAll(filepath.Join(path, "Makefile.Release"))

	return
}

func isAlreadyCached(module, path, target string, mode int) bool {
	for _, file := range cgoFileNames(module, path, target, mode) {
		file = filepath.Join(path, file)
		if utils.ExistsFile(file) {
			switch target {
			case "darwin", "linux", "windows":
				//TODO msys pkg-config mxe brew
				return strings.Contains(utils.Load(file), utils.QT_DIR()) || strings.Contains(utils.Load(file), utils.QT_DARWIN_DIR())
			case "android":
				return strings.Contains(utils.Load(file), utils.QT_DIR()) && strings.Contains(utils.Load(file), utils.ANDROID_NDK_DIR())
			case "ios", "ios-simulator":
				return strings.Contains(utils.Load(file), utils.QT_DIR()) || strings.Contains(utils.Load(file), utils.QT_DARWIN_DIR())
			case "sailfish", "sailfish-emulator", "asteroid":
			case "rpi1", "rpi2", "rpi3":
			}
		}
	}
	return false
}

func createProject(module, path string, mode int) {
	var out []string

	switch {
	case mode == RCC:
		out = []string{"Core"}
	case mode == MOC, module == "build_ios":
		out = parser.LibDeps[module]
	case mode == MINIMAL, mode == NONE:
		out = append([]string{module}, parser.LibDeps[module]...)
	}

	for i, v := range out {
		if v == "Speech" {
			out[i] = "TextToSpeech"
		}
		out[i] = strings.ToLower(out[i])
	}

	utils.Save(filepath.Join(path, "..", fmt.Sprintf("%v.pro", strings.ToLower(module))), fmt.Sprintf("QT += %v", strings.Join(out, " ")))
}

func createMakefile(module, path, target string, mode int) {
	cmd := exec.Command(utils.ToolPath("qmake", target), filepath.Join(path, "..", fmt.Sprintf("%v.pro", strings.ToLower(module))))
	cmd.Dir = path
	switch target {
	case "darwin":
		cmd.Args = append(cmd.Args, []string{"-spec", "macx-clang", "CONFIG+=x86_64"}...)
	case "windows":
		cmd.Args = append(cmd.Args, []string{"-spec", "win32-g++"}...)
	case "linux":
		cmd.Args = append(cmd.Args, []string{"-spec", "linux-g++"}...)
	case "ios":
		cmd.Args = append(cmd.Args, []string{"-spec", "macx-ios-clang", "CONFIG+=release", "CONFIG+=iphoneos", "CONFIG+=device"}...)
	case "ios-simulator":
		cmd.Args = append(cmd.Args, []string{"-spec", "macx-ios-clang", "CONFIG+=release", "CONFIG+=iphonesimulator", "CONFIG+=simulator"}...)
	case "android":
		cmd.Args = append(cmd.Args, []string{"-spec", "android-g++"}...)
		cmd.Env = []string{fmt.Sprintf("ANDROID_NDK_ROOT=%v", utils.ANDROID_NDK_DIR())}
	case "sailfish", "sailfish-emulator":
		cmd.Args = append(cmd.Args, []string{"-spec", "linux-g++"}...)
		cmd.Env = []string{
			"MER_SSH_PORT=2222",
			fmt.Sprintf("MER_SSH_PRIVATE_KEY=%v", filepath.Join(utils.SAILFISH_DIR(), "vmshare", "ssh", "private_keys", "engine", "mersdk")),
			fmt.Sprintf("MER_SSH_PROJECT_PATH=%v", cmd.Dir),
			fmt.Sprintf("MER_SSH_SDK_TOOLS=%v/.config/SailfishOS-SDK/mer-sdk-tools/MerSDK/SailfishOS-armv7hl", os.Getenv("HOME")),
			fmt.Sprintf("MER_SSH_SHARED_HOME=%v", os.Getenv("HOME")),
			fmt.Sprintf("MER_SSH_SHARED_SRC=%v", utils.MustGoPath()),
			"MER_SSH_SHARED_TARGET=/opt/SailfishOS/mersdk/targets",
			"MER_SSH_TARGET_NAME=SailfishOS-armv7hl",
			"MER_SSH_USERNAME=mersdk",
		}
	case "asteroid":
	case "rpi1", "rpi2", "rpi3":
	}

	if target == "android" && runtime.GOOS == "windows" {
		//TODO: -->
		utils.Save(filepath.Join(cmd.Dir, "qmake.bat"), fmt.Sprintf("set ANDROID_NDK_ROOT=%v\r\n%v", utils.ANDROID_NDK_DIR(), strings.Join(cmd.Args, " ")))
		cmd = exec.Command(".\\qmake.bat")
		cmd.Dir = path
		utils.RunCmdOptional(cmd, fmt.Sprintf("run qmake for %v on %v", target, runtime.GOOS))
		utils.RemoveAll(filepath.Join(cmd.Dir, "qmake.bat"))
		//<--
	} else {
		utils.RunCmdOptional(cmd, fmt.Sprintf("run qmake for %v on %v", target, runtime.GOOS))
	}

	utils.RemoveAll(filepath.Join(path, "..", fmt.Sprintf("%v.pro", strings.ToLower(module))))
	utils.RemoveAll(filepath.Join(path, ".qmake.stash"))
	switch target {
	case "darwin":
	case "windows":
		for _, suf := range []string{"_plugin_import", "_qml_plugin_import"} {
			pPath := filepath.Join(path, fmt.Sprintf("%v%v.cpp", strings.ToLower(module), suf))
			if utils.QT_MXE_STATIC() && utils.ExistsFile(pPath) {
				if content := utils.Load(pPath); !strings.Contains(content, "+build windows") {
					utils.Save(pPath, "// +build windows\n"+content)
				}
			}
			if mode == MOC || mode == RCC {
				utils.RemoveAll(pPath)
			}
		}
		for _, n := range []string{"Makefile", "Makefile.Debug", "release", "debug"} {
			utils.RemoveAll(filepath.Join(path, n))
		}
	case "linux":
	case "ios", "ios-simulator":
		for _, suf := range []string{"_plugin_import", "_qml_plugin_import"} {
			pPath := filepath.Join(path, fmt.Sprintf("%v%v.cpp", strings.ToLower(module), suf))
			if utils.QT_VERSION_MAJOR() == "5.9" && utils.ExistsFile(pPath) {
				if content := utils.Load(pPath); !strings.Contains(content, "+build ios") {
					utils.Save(pPath, "// +build ios\n"+utils.Load(pPath))
				}
			}
			if utils.QT_VERSION_MAJOR() != "5.9" || mode == MOC || mode == RCC {
				utils.RemoveAll(pPath)
			}
		}
		for _, n := range []string{"Info.plist", "qt.conf"} {
			utils.RemoveAll(filepath.Join(path, n))
		}
		utils.RemoveAll(filepath.Join(path, fmt.Sprintf("%v.xcodeproj", strings.ToLower(module))))
	case "android":
		utils.RemoveAll(filepath.Join(path, fmt.Sprintf("android-lib%v.so-deployment-settings.json", strings.ToLower(module))))
	case "sailfish", "sailfish-emulator":
	case "asteroid":
	case "rpi1", "rpi2", "rpi3":
	}
}

func createCgo(module, path, target string, mode int, ipkg string) string {
	bb := new(bytes.Buffer)
	defer bb.Reset()

	guards := "// +build "
	switch target {
	case "darwin":
		guards += "!ios"
	case "android":
		guards += "android"
	case "ios", "ios-simulator":
		guards += "ios"
	case "sailfish", "sailfish-emulator":
		guards += strings.Replace(target, "-", "_", -1)
	case "asteroid":
	case "rpi1", "rpi2", "rpi3":
	}
	switch mode {
	case NONE:
		if len(guards) > 10 {
			guards += ","
		}
		guards += "!minimal"
	case MINIMAL:
		if len(guards) > 10 {
			guards += ","
		}
		guards += "minimal"
	}
	if len(guards) > 10 {
		bb.WriteString(guards + "\n\n")
	}

	pkg := strings.ToLower(module)
	if mode == MOC {
		pkg = ipkg
	}
	fmt.Fprintf(bb, "package %v\n\n/*\n", pkg)

	//

	file := "Makefile"
	if target == "windows" {
		file += ".Release"
	}
	content := utils.Load(filepath.Join(path, file))

	for _, l := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(l, "CFLAGS"):
			fmt.Fprintf(bb, "#cgo CFLAGS: %v\n", strings.Split(l, " = ")[1])
		case strings.HasPrefix(l, "CXXFLAGS"), strings.HasPrefix(l, "INCPATH"):
			fmt.Fprintf(bb, "#cgo CXXFLAGS: %v\n", strings.Split(l, " = ")[1])
		case strings.HasPrefix(l, "LFLAGS"), strings.HasPrefix(l, "LIBS"):
			if target == "windows" && !utils.QT_MXE_STATIC() {
				pFix := []string{
					filepath.Join(utils.QT_DIR(), utils.QT_VERSION_MAJOR(), "mingw53_32"),
					filepath.Join(utils.QT_MXE_DIR(), "usr", utils.QT_MXE_TRIPLET(), "qt5"),
					utils.QT_MSYS2_DIR(),
				}
				for _, pFix := range pFix {
					pFix = strings.Replace(filepath.Join(pFix, "lib", "lib"), "\\", "/", -1)
					if strings.Contains(l, pFix) {
						var cleaned []string
						for _, s := range strings.Split(l, " ") {
							if strings.HasPrefix(s, pFix) && (strings.HasSuffix(s, ".a") || strings.HasSuffix(s, ".dll")) {
								s = strings.Replace(s, pFix, "-l", -1)
								s = strings.TrimSuffix(s, ".a")
								s = strings.TrimSuffix(s, ".dll")
							}
							cleaned = append(cleaned, s)
						}
						l = strings.Join(cleaned, " ")
					}
				}
			}
			fmt.Fprintf(bb, "#cgo LDFLAGS: %v\n", strings.Split(l, " = ")[1])
		}
	}

	switch target {
	case "android":
		fmt.Fprint(bb, "#cgo LDFLAGS: -Wl,--allow-shlib-undefined\n")
	case "windows":
		fmt.Fprint(bb, "#cgo LDFLAGS: -Wl,--allow-multiple-definition\n")
	}

	fmt.Fprint(bb, "*/\nimport \"C\"\n")

	tmp := bb.String()

	switch target {
	case "ios":
		tmp = strings.Replace(tmp, "$(EXPORT_ARCH_ARGS)", "-arch arm64", -1)
		tmp = strings.Replace(tmp, "$(EXPORT_QMAKE_XARCH_CFLAGS)", "", -1)
		tmp = strings.Replace(tmp, "$(EXPORT_QMAKE_XARCH_LFLAGS)", "", -1)
	case "ios-simulator":
		tmp = strings.Replace(tmp, "$(EXPORT_ARCH_ARGS)", "-arch x86_64", -1)
		tmp = strings.Replace(tmp, "$(EXPORT_QMAKE_XARCH_CFLAGS)", "", -1)
		tmp = strings.Replace(tmp, "$(EXPORT_QMAKE_XARCH_LFLAGS)", "", -1)
	case "android":
		tmp = strings.Replace(tmp, fmt.Sprintf("-Wl,-soname,lib%v.so", strings.ToLower(module)), "", -1)
		tmp = strings.Replace(tmp, "-shared", "", -1)
	}

	for _, variable := range []string{"DEFINES", "SUBLIBS", "EXPORT_QMAKE_XARCH_CFLAGS", "EXPORT_QMAKE_XARCH_LFLAGS", "EXPORT_ARCH_ARGS", "-fvisibility=hidden", "-fembed-bitcode"} {
		for _, l := range strings.Split(content, "\n") {
			if strings.HasPrefix(l, variable+" ") {
				if strings.Contains(l, "-DQT_TESTCASE_BUILDDIR") {
					l = strings.Split(l, "-DQT_TESTCASE_BUILDDIR")[0]
				}
				tmp = strings.Replace(tmp, fmt.Sprintf("$(%v)", variable), strings.Split(l, " = ")[1], -1)
			}
		}
		tmp = strings.Replace(tmp, fmt.Sprintf("$(%v)", variable), "", -1)
		tmp = strings.Replace(tmp, variable, "", -1)
	}
	tmp = strings.Replace(tmp, "\\", "/", -1)

	if module == "build_ios" {
		return tmp
	}

	for _, file := range cgoFileNames(module, path, target, mode) {
		switch target {
		case "windows":
			if utils.UseMsys2() && utils.QT_MSYS2_ARCH() == "amd64" {
				tmp = strings.Replace(tmp, " -Wa,-mbig-obj ", " ", -1)
			}
			if (utils.UseMsys2() && utils.QT_MSYS2_ARCH() == "amd64") || utils.QT_MXE_ARCH() == "amd64" {
				tmp = strings.Replace(tmp, " -Wl,-s ", " ", -1)
			}
		case "ios":
			if strings.HasSuffix(file, "darwin_arm.go") {
				tmp = strings.Replace(tmp, "arm64", "armv7", -1)
			}
		case "ios-simulator":
			if strings.HasSuffix(file, "darwin_386.go") {
				tmp = strings.Replace(tmp, "x86_64", "i386", -1)
			}
		}
		utils.Save(filepath.Join(path, file), tmp)
	}

	return ""
}

func cgoFileNames(module, path, target string, mode int) []string {
	var pFix string
	switch mode {
	case RCC:
		pFix = "rcc_"
	case MOC:
		pFix = "moc_"
	case MINIMAL:
		pFix = "minimal_"
	}

	var sFixes []string
	switch target {
	case "darwin":
		sFixes = []string{"darwin_amd64"}
	case "linux":
		sFixes = []string{"linux_amd64"}
	case "windows":
		if utils.QT_MXE_ARCH() == "amd64" || (utils.UseMsys2() && utils.QT_MSYS2_ARCH() == "amd64") {
			sFixes = []string{"windows_amd64"}
		} else {
			sFixes = []string{"windows_386"}
		}
	case "android":
		sFixes = []string{"linux_arm"}
	case "ios":
		sFixes = []string{"darwin_arm64", "darwin_arm"}
	case "ios-simulator":
		sFixes = []string{"darwin_amd64", "darwin_386"}
	case "sailfish":
		sFixes = []string{"linux_arm"}
	case "sailfish-emulator":
		sFixes = []string{"linux_386"}
	case "asteroid":
		sFixes = []string{"linux_arm"}
	case "rpi1", "rpi2", "rpi3":
		sFixes = []string{"linux_arm"}
	}

	var o []string
	for _, sFix := range sFixes {
		o = append(o, fmt.Sprintf("%vcgo_%v_%v.go", pFix, strings.Replace(target, "-", "_", -1), sFix))
	}
	return o
}
