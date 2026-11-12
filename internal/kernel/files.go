package kernel

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"nativejet/internal/model"
	"nativejet/internal/storage"
)

func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// The descriptor and the directory entry must identify the same regular file.
// Unix additionally opens with O_NOFOLLOW/O_NONBLOCK, including against FIFO swaps.
func openRegular(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	if filepath.Base(name) != name || name == "." {
		return nil, nil, errors.New("无效文件名")
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, errors.New("文件必须为普通文件，不能为符号链接")
	}
	f, err := root.OpenFile(name, kernelOpenFlags, 0)
	if err != nil {
		return nil, nil, err
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = f.Close()
		return nil, nil, errors.New("打开时文件身份发生变化")
	}
	return f, after, nil
}

func unchanged(root *os.Root, name string, f *os.File, before os.FileInfo) error {
	after, err := f.Stat()
	if err != nil {
		return err
	}
	entry, err := root.Lstat(name)
	if err != nil || !entry.Mode().IsRegular() || !os.SameFile(entry, after) ||
		!os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("读取时文件发生变化，请重新扫描")
	}
	return nil
}

func readRegular(root *os.Root, name string, limit int64) ([]byte, os.FileInfo, error) {
	f, info, err := openRegular(root, name)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	if info.Size() > limit {
		return nil, nil, errors.New("文件过大")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > limit {
		return nil, nil, errors.New("文件过大")
	}
	if err := unchanged(root, name, f, info); err != nil {
		return nil, nil, err
	}
	return data, info, nil
}

func managedRoot(path string, create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("受管理内核目录必须为真实目录")
	}
	if err := checkManagedInfo(info, false); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, errors.New("受管理内核目录身份发生变化")
	}
	return root, nil
}

func managedChild(root *os.Root, name string, create bool) (*os.Root, error) {
	if create {
		if err := root.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("受管理内核子目录不能为符号链接")
	}
	if err := checkManagedInfo(info, false); err != nil {
		return nil, err
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := child.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = child.Close()
		return nil, errors.New("受管理内核子目录身份发生变化")
	}
	return child, nil
}

// Every write stays relative to an already-open, checked directory.
func atomicWrite(root *os.Root, name string, data []byte, mode os.FileMode) error {
	tmp := ".tmp-" + storage.ID()
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return root.Rename(tmp, name)
}

func (m *Manager) installSnapshot(item model.Kernel, data []byte) (model.Kernel, error) {
	root, err := managedRoot(m.Installed, true)
	if err != nil {
		return item, err
	}
	defer root.Close()
	child, err := managedChild(root, item.ID, true)
	if err != nil {
		return item, err
	}
	defer child.Close()
	if err = atomicWrite(child, Executable, data, executableMode); err != nil {
		return item, err
	}
	item.Path = filepath.Join(m.Installed, item.ID, Executable)
	item.Installed = true
	return item, nil
}

func (m *Manager) VerifyInstalled(item model.Kernel) error {
	expected := filepath.Join(m.Installed, item.ID, Executable)
	if filepath.Clean(item.Path) != filepath.Clean(expected) {
		return errors.New("内核路径不在受管理目录内")
	}
	f, err := OpenVerified(expected, item.ID)
	if err == nil {
		err = f.Close()
	}
	return err
}

// Keep this descriptor open through process creation. Only the managed,
// permission-checked executable is eligible for execution, and its hash is
// checked again immediately before cmd.Start, after controller I/O completes.
func OpenVerified(path, id string) (*os.File, error) {
	if !validID(id) || filepath.Base(path) != Executable || filepath.Base(filepath.Dir(path)) != id {
		return nil, errors.New("无效的受管理内核路径或指纹")
	}
	root, err := managedRoot(filepath.Dir(filepath.Dir(path)), false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	child, err := managedChild(root, id, false)
	if err != nil {
		return nil, err
	}
	defer child.Close()
	f, info, err := openRegular(child, Executable)
	if err != nil {
		return nil, err
	}
	check := func() error {
		if err := checkManagedInfo(info, true); err != nil {
			return err
		}
		hash, err := hashBinary(f)
		if err != nil {
			return err
		}
		if hash != id {
			return errors.New("受管理的内核文件身份发生变化")
		}
		return unchanged(child, Executable, f, info)
	}
	if err := check(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("内核校验失败: %w", err)
	}
	return f, nil
}
