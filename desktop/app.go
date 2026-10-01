package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	service "github.com/harveyxiacn/cc-router/internal/desktop"
	"github.com/harveyxiacn/cc-router/internal/localdata"
	"github.com/harveyxiacn/cc-router/internal/update"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App binds native dialogs and forwards account operations to the shared service.
type App struct {
	ctx                 context.Context
	backend             *service.Service
	initializationError string
	openBrowser         func(context.Context, string)
	quitRequested       sync.Once
}

func NewApp(cliPath string) *App {
	a := &App{}
	backend, err := service.New(cliPath)
	if err != nil {
		a.initializationError = err.Error()
	} else {
		a.backend = backend
	}
	return a
}
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.backend != nil {
		if err := a.backend.StartUpdates(ctx); errors.Is(err, update.ErrRecoveryStarted) || errors.Is(err, update.ErrBusy) {
			wailsruntime.Quit(ctx)
		}
	}
}
func (a *App) shutdown() {
	if a.backend != nil {
		a.backend.CloseUpdates()
	}
}
func (a *App) GetUpdateStatus() (service.UpdateInfo, error) {
	s, err := a.ready()
	if err != nil {
		return service.UpdateInfo{}, err
	}
	info, err := s.GetUpdateStatus()
	if err == nil && info.Phase == "recovering" && a.ctx != nil {
		a.quitForUpdate()
	}
	return info, err
}
func (a *App) CheckForUpdates() (service.UpdateInfo, error) {
	s, err := a.ready()
	if err != nil {
		return service.UpdateInfo{}, err
	}
	return s.CheckForUpdates()
}
func (a *App) SetAutoUpdate(enabled bool) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.SetAutoUpdate(enabled)
}
func (a *App) SetUpdateIdle(idle bool) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.SetUpdateIdle(idle)
}
func (a *App) PrepareUpdate() error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.PrepareUpdate()
}
func (a *App) ReportReady() error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.ReportReady()
}
func (a *App) ApplyUpdate() error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	if a.ctx == nil {
		return errors.New("桌面窗口尚未就绪，请重新启动后更新")
	}
	if err = s.ApplyUpdate(); err != nil {
		return err
	}
	a.quitForUpdate()
	return nil
}
func (a *App) RollbackUpdate() error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	if a.ctx == nil {
		return errors.New("桌面窗口尚未就绪，请重新启动后回退")
	}
	if err = s.RollbackUpdate(); err != nil {
		return err
	}
	a.quitForUpdate()
	return nil
}
func (a *App) quitForUpdate() {
	a.quitRequested.Do(func() { time.AfterFunc(250*time.Millisecond, func() { wailsruntime.Quit(a.ctx) }) })
}
func (a *App) OpenReleasePage() error {
	if a.ctx == nil {
		return errors.New("桌面链接尚未就绪，请从官方 GitHub 发行页查看更新")
	}
	open := a.openBrowser
	if open == nil {
		open = wailsruntime.BrowserOpenURL
	}
	open(a.ctx, update.ReleasePage)
	return nil
}
func (a *App) ready() (*service.Service, error) {
	if a.backend == nil {
		return nil, fmt.Errorf("桌面后端初始化失败：%s。请检查 CCR_HOME 是否为可写的绝对目录，再重新启动。", a.initializationError)
	}
	return a.backend, nil
}
func (a *App) GetSnapshot(project string) (service.Snapshot, error) {
	s, err := a.ready()
	if err != nil {
		return service.Snapshot{}, err
	}
	return s.GetSnapshot(project)
}
func (a *App) CreateAccount(name, label string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.CreateAccount(name, label)
}
func (a *App) RenameAccount(name, newName, label string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.RenameAccount(name, newName, label)
}
func (a *App) RemoveAccount(name string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.RemoveAccount(name)
}
func (a *App) SetDefault(name string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.SetDefault(name)
}
func (a *App) Bind(project, name string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.Bind(project, name)
}
func (a *App) Unbind(project string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.Unbind(project)
}
func (a *App) CheckAccount(name, project string) (service.Identity, error) {
	s, err := a.ready()
	if err != nil {
		return service.Identity{}, err
	}
	return s.CheckAccount(name, project)
}
func (a *App) Diagnose(name, project string) (service.Diagnosis, error) {
	s, err := a.ready()
	if err != nil {
		return service.Diagnosis{}, err
	}
	return s.Diagnose(name, project)
}
func (a *App) CreateHandoff(project string) (service.Handoff, error) {
	s, err := a.ready()
	if err != nil {
		return service.Handoff{}, err
	}
	return s.CreateHandoff(project)
}
func (a *App) ReadHandoff(project string) (service.Handoff, error) {
	s, err := a.ready()
	if err != nil {
		return service.Handoff{}, err
	}
	return s.ReadHandoff(project)
}
func (a *App) SaveHandoff(project, content, digest string) (service.Handoff, error) {
	s, err := a.ready()
	if err != nil {
		return service.Handoff{}, err
	}
	return s.SaveHandoff(project, content, digest)
}
func (a *App) Launch(name, project, mode string, reviewed bool) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.Launch(name, project, mode, reviewed)
}
func (a *App) InstallUsage(name string, prepare, switchAt float64) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.InstallUsage(name, prepare, switchAt)
}

func (a *App) ChooseDirectory() (string, error) {
	if a.ctx == nil {
		return "", errors.New("系统文件夹选择器尚未就绪，请重新打开桌面应用")
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择工作项目文件夹", CanCreateDirectories: true})
}
func (a *App) ExportMetadata() (string, error) {
	s, err := a.ready()
	if err != nil {
		return "", err
	}
	if a.ctx == nil {
		return "", errors.New("系统文件选择器尚未就绪")
	}
	data, err := s.ExportMetadata()
	if err != nil {
		return "", err
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{Title: "导出账号元数据（不含凭据）", DefaultFilename: "cc-router-accounts.json", Filters: []wailsruntime.FileFilter{{DisplayName: "JSON 元数据", Pattern: "*.json"}}})
	if err != nil || path == "" {
		return path, err
	}
	if err := validateMetadataDestination(path, s.Store.Root); err != nil {
		return "", err
	}
	if err := writeMetadata(path, data); err != nil {
		return "", err
	}
	return path, nil
}
func validateMetadataDestination(path, dataRoot string) error {
	if !filepath.IsAbs(path) {
		return errors.New("导出目标必须为绝对文件路径")
	}
	if strings.EqualFold(filepath.Base(path), ".credentials.json") {
		return errors.New("不能导出到官方凭据文件，请另选 JSON 文件")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return errors.New("无法确定导出目录，请另选已存在的文件夹")
	}
	root, err := filepath.EvalSymlinks(dataRoot)
	if err != nil {
		return errors.New("无法确定本地配置目录，导出已停止")
	}
	target := filepath.Join(parent, filepath.Base(path))
	if runtime.GOOS == "windows" {
		target, root = strings.ToLower(target), strings.ToLower(root)
	}
	relative, err := filepath.Rel(root, target)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("不能导出到应用配置目录或官方账号目录，请另选一个文件夹")
	}
	return nil
}
func (a *App) ImportMetadata() (string, error) {
	s, err := a.ready()
	if err != nil {
		return "", err
	}
	if a.ctx == nil {
		return "", errors.New("系统文件选择器尚未就绪")
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "导入账号元数据", Filters: []wailsruntime.FileFilter{{DisplayName: "JSON 元数据", Pattern: "*.json"}}})
	if err != nil || path == "" {
		return path, err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return "", errors.New("无法打开选择的元数据目录")
	}
	defer root.Close()
	data, err := localdata.Read(root, filepath.Base(path), 1024*1024)
	if err != nil {
		return "", errors.New("导入文件不可读取、为链接，或超过 1 MiB")
	}
	if err := s.ImportMetadata(string(data)); err != nil {
		return "", err
	}
	return path, nil
}
func writeMetadata(path, data string) error {
	if !filepath.IsAbs(path) || len(data) > 1024*1024 {
		return errors.New("导出目标必须是绝对路径，数据最多 1 MiB")
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return errors.New("无法打开导出目录")
	}
	defer root.Close()
	if err := localdata.Write(root, filepath.Base(path), []byte(data)); err != nil {
		return errors.New("无法保存元数据，请选择可写的普通文件目标")
	}
	return nil
}
func resolveCLIPath() (string, error) {
	if override := os.Getenv("CC_ROUTER_CLI"); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("CC_ROUTER_CLI 必须指向绝对路径的 CC Router CLI")
		}
		return filepath.Clean(override), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", errors.New("无法确定桌面应用目录")
	}
	filename := "cc-router"
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	dir := filepath.Dir(exe)
	candidate := filepath.Join(dir, filename)
	if runtime.GOOS == "darwin" && filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			bundle := filepath.Dir(filepath.Dir(dir))
			sibling := filepath.Join(filepath.Dir(bundle), filename)
			if info, err := os.Stat(sibling); err == nil && info.Mode().IsRegular() {
				return sibling, nil
			}
		}
	}
	return candidate, nil
}
