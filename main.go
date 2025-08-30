//go:build windows
// +build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

/* ===================== 基础类型 & 常量 ===================== */

type (
	HICON     uintptr
	HINSTANCE uintptr
	HANDLE    uintptr
	DWORD     uint32
)

const (
	MAX_PATH = 260

	// SHGetFileInfo flags
	SHGFI_ICON         = 0x000000100
	SHGFI_LARGEICON    = 0x000000000
	SHGFI_SMALLICON    = 0x000000001
	SHGFI_SYSICONINDEX = 0x000004000

	// IImageList size groups
	SHIL_SMALL      = 0 // 16x16
	SHIL_LARGE      = 1 // 32x32
	SHIL_EXTRALARGE = 2 // 48x48
	SHIL_JUMBO      = 4 // 256x256

	// IImageList::GetIcon flags
	ILD_NORMAL = 0x00000000
)

/* ===================== 结构体 ===================== */

type SHFILEINFOW struct {
	HIcon         HICON
	IIcon         int32
	DwAttributes  DWORD
	SzDisplayName [MAX_PATH]uint16
	SzTypeName    [80]uint16
}

type GdiplusStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

type CLSID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

/* ===================== DLL/Proc ===================== */

var (
	modShell32 = syscall.NewLazyDLL("shell32.dll")
	modUser32  = syscall.NewLazyDLL("user32.dll")
	modGdiPlus = syscall.NewLazyDLL("gdiplus.dll")
	modOle32   = syscall.NewLazyDLL("ole32.dll")

	// Shell
	procSHGetFileInfoW = modShell32.NewProc("SHGetFileInfoW")
	procSHGetImageList = modShell32.NewProc("SHGetImageList") // Vista+ 正常可用

	// User32
	procDestroyIcon = modUser32.NewProc("DestroyIcon")

	// GDI+
	procGdiplusStartup            = modGdiPlus.NewProc("GdiplusStartup")
	procGdiplusShutdown           = modGdiPlus.NewProc("GdiplusShutdown")
	procGdipCreateBitmapFromHICON = modGdiPlus.NewProc("GdipCreateBitmapFromHICON")
	procGdipSaveImageToFile       = modGdiPlus.NewProc("GdipSaveImageToFile")
	procGdipDisposeImage          = modGdiPlus.NewProc("GdipDisposeImage")

	// OLE/COM
	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize   = modOle32.NewProc("CoUninitialize")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
)

/* ===================== GDI+ & PNG 编码 ===================== */

var CLSID_EncoderPNG = CLSID{
	Data1: 0x557cf406, Data2: 0x1a04, Data3: 0x11d3,
	Data4: [8]byte{0x9a, 0x73, 0x00, 0x00, 0xf8, 0x1e, 0xf3, 0x2e},
}

type gdiplusToken uintptr

func gdiplusStartup() (gdiplusToken, error) {
	var token gdiplusToken
	var input GdiplusStartupInput
	input.GdiplusVersion = 1
	r1, _, err := procGdiplusStartup.Call(
		uintptr(unsafe.Pointer(&token)),
		uintptr(unsafe.Pointer(&input)),
		0,
	)
	if r1 != 0 {
		if err != syscall.Errno(0) {
			return 0, err
		}
		return 0, fmt.Errorf("GdiplusStartup failed, status=%d", r1)
	}
	return token, nil
}
func gdiplusShutdown(token gdiplusToken) { procGdiplusShutdown.Call(uintptr(token)) }

func saveIconToPNG(hicon HICON, outPath string) error {
	token, err := gdiplusStartup()
	if err != nil {
		return err
	}
	defer gdiplusShutdown(token)

	var img uintptr
	r1, _, errCall := procGdipCreateBitmapFromHICON.Call(uintptr(hicon), uintptr(unsafe.Pointer(&img)))
	if r1 != 0 || img == 0 {
		if errCall != syscall.Errno(0) {
			return fmt.Errorf("GdipCreateBitmapFromHICON failed: %v", errCall)
		}
		return fmt.Errorf("GdipCreateBitmapFromHICON failed, status=%d", r1)
	}
	defer procGdipDisposeImage.Call(img)

	outW, _ := syscall.UTF16PtrFromString(outPath)
	r2, _, err2 := procGdipSaveImageToFile.Call(
		img,
		uintptr(unsafe.Pointer(outW)),
		uintptr(unsafe.Pointer(&CLSID_EncoderPNG)),
		0,
	)
	if r2 != 0 {
		if err2 != syscall.Errno(0) {
			return fmt.Errorf("GdipSaveImageToFile failed: %v", err2)
		}
		return fmt.Errorf("GdipSaveImageToFile failed, status=%d", r2)
	}
	return nil
}

/* ===================== IImageList（只用到 GetIcon） ===================== */

var IID_IImageList = GUID{0x46EB5926, 0x582E, 0x4017, [8]byte{0x9F, 0xDF, 0xE8, 0x99, 0x8D, 0xAA, 0x09, 0x50}}

type IImageListVtbl struct {
	QueryInterface  uintptr
	AddRef          uintptr
	Release         uintptr
	Add             uintptr
	ReplaceIcon     uintptr
	SetOverlayImage uintptr
	Replace         uintptr
	AddMasked       uintptr
	Draw            uintptr
	Remove          uintptr
	GetIcon         uintptr // 我们只用这个
	// vtable 后续成员省略
}
type IImageList struct{ LpVtbl *IImageListVtbl }

func (iml *IImageList) GetIcon(i int32, flags uint32, phicon *HICON) int32 {
	ret, _, _ := syscall.SyscallN(iml.LpVtbl.GetIcon,
		uintptr(unsafe.Pointer(iml)),
		uintptr(i),
		uintptr(flags),
		uintptr(unsafe.Pointer(phicon)),
	)
	return int32(ret) // S_OK = 0
}
func (iml *IImageList) Release() {
	_, _, _ = syscall.SyscallN(iml.LpVtbl.Release, uintptr(unsafe.Pointer(iml)))
}

/* ===================== SHGetImageList + SHGetFileInfo(索引) ===================== */

func getSysIconIndex(path string) (int32, error) {
	p, _ := syscall.UTF16PtrFromString(path)
	var sfi SHFILEINFOW
	r1, _, callErr := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(unsafe.Pointer(&sfi)),
		unsafe.Sizeof(sfi),
		uintptr(SHGFI_SYSICONINDEX),
	)
	if r1 == 0 {
		if callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return 0, errors.New("SHGetFileInfoW (SYSICONINDEX) failed")
	}
	return sfi.IIcon, nil
}

func getImageList(sizeGroup int32) (*IImageList, error) {
	var p unsafe.Pointer
	r1, _, err := procSHGetImageList.Call(
		uintptr(sizeGroup),
		uintptr(unsafe.Pointer(&IID_IImageList)),
		uintptr(unsafe.Pointer(&p)),
	)
	if r1 != 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, fmt.Errorf("SHGetImageList failed: 0x%x", r1)
	}
	return (*IImageList)(p), nil
}

func getIconWithSizeByIndex(index int32, sizeGroup int32) (HICON, error) {
	iml, err := getImageList(sizeGroup)
	if err != nil {
		return 0, err
	}
	defer iml.Release()

	var h HICON
	if iml.GetIcon(index, ILD_NORMAL, &h) != 0 || h == 0 {
		return 0, errors.New("IImageList.GetIcon failed")
	}
	return h, nil
}

/* ===================== 旧的 SHGetFileInfo（回退用） ===================== */

func SHGetFileIconFallback(path string, large bool) (HICON, error) {
	ptr, _ := syscall.UTF16PtrFromString(path)
	var sfi SHFILEINFOW
	flags := uint32(SHGFI_ICON)
	if large {
		flags |= SHGFI_LARGEICON
	} else {
		flags |= SHGFI_SMALLICON
	}
	r1, _, callErr := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(ptr)),
		0,
		uintptr(unsafe.Pointer(&sfi)),
		unsafe.Sizeof(sfi),
		uintptr(flags),
	)
	if r1 == 0 || sfi.HIcon == 0 {
		if callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return 0, errors.New("SHGetFileInfoW failed")
	}
	return sfi.HIcon, nil
}

func DestroyIcon(h HICON) {
	if h != 0 {
		procDestroyIcon.Call(uintptr(h))
	}
}

/* ===================== 解析 .lnk（IShellLinkW + IPersistFile） ===================== */

const (
	COINIT_APARTMENTTHREADED = 0x2
	CLSCTX_INPROC_SERVER     = 0x1
	SLGP_SHORTPATH           = 0x1
	SLGP_UNCPRIORITY         = 0x2
	SLGP_RAWPATH             = 0x4
)

var (
	CLSID_ShellLink  = GUID{0x00021401, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	IID_IShellLinkW  = GUID{0x000214F9, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	IID_IPersistFile = GUID{0x0000010b, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

type IUnknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type IShellLinkWVtbl struct {
	QueryInterface      uintptr
	AddRef              uintptr
	Release             uintptr
	GetPath             uintptr
	GetIDList           uintptr
	SetIDList           uintptr
	GetDescription      uintptr
	SetDescription      uintptr
	GetWorkingDirectory uintptr
	SetWorkingDirectory uintptr
	GetArguments        uintptr
	SetArguments        uintptr
	GetHotkey           uintptr
	SetHotkey           uintptr
	GetShowCmd          uintptr
	SetShowCmd          uintptr
	GetIconLocation     uintptr
	SetIconLocation     uintptr
	SetRelativePath     uintptr
	Resolve             uintptr
	SetPath             uintptr
}
type IShellLinkW struct{ LpVtbl *IShellLinkWVtbl }

func (sl *IShellLinkW) Release() { syscall.SyscallN(sl.LpVtbl.Release, uintptr(unsafe.Pointer(sl))) }
func (sl *IShellLinkW) GetPath(buf *uint16, bufLen int, findData *WIN32_FIND_DATAW, flags uint32) error {
	ret, _, _ := syscall.SyscallN(sl.LpVtbl.GetPath,
		uintptr(unsafe.Pointer(sl)),
		uintptr(unsafe.Pointer(buf)),
		uintptr(uint32(bufLen)),
		uintptr(unsafe.Pointer(findData)),
		uintptr(flags),
	)
	if ret != 0 {
		return syscall.Errno(ret)
	} // S_OK = 0
	return nil
}

type IPersistFileVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetClassID     uintptr
	IsDirty        uintptr
	Load           uintptr
	Save           uintptr
	SaveCompleted  uintptr
	GetCurFile     uintptr
}
type IPersistFile struct{ LpVtbl *IPersistFileVtbl }

func (pf *IPersistFile) Release() { syscall.SyscallN(pf.LpVtbl.Release, uintptr(unsafe.Pointer(pf))) }
func (pf *IPersistFile) Load(fileName *uint16, mode uint32) error {
	ret, _, _ := syscall.SyscallN(
		pf.LpVtbl.Load,
		uintptr(unsafe.Pointer(pf)),
		uintptr(unsafe.Pointer(fileName)),
		uintptr(mode),
	)
	if ret != 0 {
		return syscall.Errno(ret)
	} // S_OK=0
	return nil
}

type FILETIME struct{ DwLowDateTime, DwHighDateTime uint32 }
type WIN32_FIND_DATAW struct {
	DwFileAttributes   uint32
	FtCreationTime     FILETIME
	FtLastAccessTime   FILETIME
	FtLastWriteTime    FILETIME
	NFileSizeHigh      uint32
	NFileSizeLow       uint32
	DwReserved0        uint32
	DwReserved1        uint32
	CFileName          [MAX_PATH]uint16
	CAlternateFileName [14]uint16
}

func coInitialize() error {
	r, _, err := procCoInitializeEx.Call(0, uintptr(COINIT_APARTMENTTHREADED))
	if r != 0 && r != 1 { // S_OK=0, S_FALSE=1
		if err != syscall.Errno(0) {
			return err
		}
		return fmt.Errorf("CoInitializeEx failed: 0x%x", r)
	}
	return nil
}
func coUninit() { procCoUninitialize.Call() }

func coCreateInstance(clsid, iid *GUID, out **uintptr) error {
	var pUnk *uintptr
	r, _, err := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)),
		0,
		uintptr(CLSCTX_INPROC_SERVER),
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&pUnk)),
	)
	if r != 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return fmt.Errorf("CoCreateInstance failed: 0x%x", r)
	}
	*out = pUnk
	return nil
}
func queryInterface(pThis *uintptr, iid *GUID, ppv **uintptr) error {
	vtbl := *(**IUnknownVtbl)(unsafe.Pointer(pThis))
	r, _, _ := syscall.SyscallN(
		vtbl.QueryInterface,
		uintptr(unsafe.Pointer(pThis)),
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(ppv)),
	)
	if r != 0 {
		return fmt.Errorf("QueryInterface failed: 0x%x", r)
	}
	return nil
}

func resolveShortcut(lnk string) (string, error) {
	if err := coInitialize(); err != nil {
		return "", err
	}
	defer coUninit()

	var pSLRaw *uintptr
	if err := coCreateInstance(&CLSID_ShellLink, &IID_IShellLinkW, &pSLRaw); err != nil {
		return "", err
	}
	sl := (*IShellLinkW)(unsafe.Pointer(pSLRaw))
	defer sl.Release()

	var pPfRaw *uintptr
	if err := queryInterface(pSLRaw, &IID_IPersistFile, &pPfRaw); err != nil {
		return "", err
	}
	pf := (*IPersistFile)(unsafe.Pointer(pPfRaw))
	defer pf.Release()

	pathW, _ := syscall.UTF16PtrFromString(lnk)
	if err := pf.Load(pathW, 0); err != nil {
		return "", fmt.Errorf("IPersistFile.Load failed: %w", err)
	}

	var findData WIN32_FIND_DATAW
	buf := make([]uint16, MAX_PATH)
	if err := sl.GetPath(&buf[0], len(buf), &findData, SLGP_RAWPATH|SLGP_UNCPRIORITY); err != nil {
		if err2 := sl.GetPath(&buf[0], len(buf), &findData, SLGP_SHORTPATH); err2 != nil {
			return "", fmt.Errorf("IShellLinkW.GetPath failed: %v / %v", err, err2)
		}
	}
	return syscall.UTF16ToString(buf), nil
}

/* ===================== 封装：按尺寸取 HICON ===================== */

type IconSize int

const (
	SizeS  IconSize = iota // 16
	SizeM                  // 32
	SizeL                  // 48
	SizeXL                 // 256
)

func pickSizeGroup(sz IconSize) int32 {
	switch sz {
	case SizeS:
		return SHIL_SMALL
	case SizeM:
		return SHIL_LARGE
	case SizeL:
		return SHIL_EXTRALARGE
	case SizeXL:
		return SHIL_JUMBO
	default:
		return SHIL_EXTRALARGE
	}
}

func getIconForPathWithSize(path string, sz IconSize) (HICON, error) {
	// 先拿系统图标索引
	index, err := getSysIconIndex(path)
	if err == nil {
		// 再从对应尺寸的系统图像表取 HICON
		h, err2 := getIconWithSizeByIndex(index, pickSizeGroup(sz))
		if err2 == nil && h != 0 {
			return h, nil
		}
	}
	// 失败兜底（只能大/小）
	switch sz {
	case SizeS:
		return SHGetFileIconFallback(path, false)
	default:
		return SHGetFileIconFallback(path, true)
	}
}

/* ===================== 主程序 ===================== */

func main() {
	// 命令行尺寸参数
	flagS := flag.Bool("s", false, "保存 16x16 小图标")
	flagM := flag.Bool("m", false, "保存 32x32 中图标")
	flagL := flag.Bool("l", false, "保存 48x48 大图标")
	flagXL := flag.Bool("xl", false, "保存 256x256 超大图标（若系统支持）")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("用法: icon_dump.exe [-s|-m|-l|-xl] <文件/文件夹/快捷方式(.lnk)路径>")
		os.Exit(2)
	}

	// 选择尺寸：优先级 xl > l > m > s
	size := SizeL
	if *flagXL {
		size = SizeXL
	} else if *flagL {
		size = SizeL
	} else if *flagM {
		size = SizeM
	} else if *flagS {
		size = SizeS
	}

	in := flag.Arg(0)
	use := in

	// 解析 .lnk
	if strings.EqualFold(filepath.Ext(in), ".lnk") {
		target, err := resolveShortcut(in)
		if err != nil || target == "" {
			fmt.Println("解析快捷方式失败：", err)
			os.Exit(1)
		}
		abs, _ := filepath.Abs(target)
		fmt.Printf("Shortcut: %q\n", in)
		fmt.Printf("Target  : %q\n", abs)
		use = target
	}

	// 按尺寸取图标
	h, err := getIconForPathWithSize(use, size)
	if err != nil || h == 0 {
		fmt.Println("获取图标失败：", err)
		os.Exit(1)
	}
	defer DestroyIcon(h)

	// 输出文件名
	base := filepath.Base(use)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	if name == "" {
		name = "icon"
	}

	var suffix string
	switch size {
	case SizeS:
		suffix = "_16"
	case SizeM:
		suffix = "_32"
	case SizeL:
		suffix = "_48"
	case SizeXL:
		suffix = "_256"
	}
	out := name + suffix + ".png"

	if err := saveIconToPNG(h, out); err != nil {
		fmt.Println("保存 PNG 失败：", err)
		os.Exit(1)
	}
	fmt.Println("已保存：", out)
}
