package workers

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/dop251/goja"
)

// WriteFS is an optional write surface a mounted [fs.FS] may implement.
// Without it, write methods fail with EROFS. The Binding never calls os.
type WriteFS interface {
	WriteFile(name string, data []byte, perm fs.FileMode) error
	Mkdir(name string, perm fs.FileMode) error
	Remove(name string) error
}

// RealpathFS is an optional symlink-resolving surface a mounted [fs.FS]
// may implement. Without it, realpath returns the cleaned guest path.
type RealpathFS interface {
	Realpath(name string) (string, error)
}

var (
	errBadPathType = errors.New("workers: fs path type")
	errPathNUL     = errors.New("workers: fs path NUL")
	errPathEscape  = errors.New("workers: fs path escape")
	errIsDir       = errors.New("workers: is a directory")
	errNotDir      = errors.New("workers: not a directory")
	errReadOnly    = errors.New("workers: read-only fs")
)

// nodeFSBinding materializes guest require("fs") / require("node:fs").
type nodeFSBinding struct{}

var _ Binding = nodeFSBinding{}

func (nodeFSBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"fs", "node:fs"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeFS(iso), nil
}

type nodeFS struct {
	iso  *Isolate
	fsys fs.FS
}

func newNodeFS(iso *Isolate) *goja.Object {
	n := &nodeFS{iso: iso, fsys: iso.opts.FS}
	obj := iso.vm.NewObject()
	consts := nodeFSConstants(iso.vm)
	mustSet(obj, "constants", consts)
	for _, name := range []string{"F_OK", "R_OK", "W_OK", "X_OK"} {
		mustSet(obj, name, consts.Get(name))
	}

	mustSet(obj, "access", n.jsAccess)
	mustSet(obj, "accessSync", n.jsAccessSync)
	mustSet(obj, "appendFile", n.jsAppendFile)
	mustSet(obj, "appendFileSync", n.jsAppendFileSync)
	mustSet(obj, "chmod", n.jsStub1("chmod", false))
	mustSet(obj, "chmodSync", n.jsStub1("chmod", true))
	mustSet(obj, "chown", n.jsStub1("chown", false))
	mustSet(obj, "chownSync", n.jsStub1("chown", true))
	mustSet(obj, "copyFile", n.jsCopyFile)
	mustSet(obj, "copyFileSync", n.jsCopyFileSync)
	mustSet(obj, "lchown", n.jsStub1("lchown", false))
	mustSet(obj, "lchownSync", n.jsStub1("lchown", true))
	mustSet(obj, "link", n.jsStub2("link", "existingPath", "newPath", false))
	mustSet(obj, "linkSync", n.jsStub2("link", "existingPath", "newPath", true))
	mustSet(obj, "lstat", n.jsLstat)
	mustSet(obj, "lstatSync", n.jsLstatSync)
	mustSet(obj, "mkdir", n.jsMkdir)
	mustSet(obj, "mkdirSync", n.jsMkdirSync)
	mustSet(obj, "open", n.jsOpen)
	mustSet(obj, "openSync", n.jsOpenSync)
	mustSet(obj, "close", n.jsClose)
	mustSet(obj, "closeSync", n.jsCloseSync)
	mustSet(obj, "readFile", n.jsReadFile)
	mustSet(obj, "readFileSync", n.jsReadFileSync)
	mustSet(obj, "readdir", n.jsReaddir)
	mustSet(obj, "readdirSync", n.jsReaddirSync)
	mustSet(obj, "readlink", n.jsReadlink)
	mustSet(obj, "readlinkSync", n.jsReadlinkSync)
	n.setNative(obj, "realpath", n.jsRealpath)
	n.setNative(obj, "realpathSync", n.jsRealpathSync)
	mustSet(obj, "rename", n.jsStub2("rename", "oldPath", "newPath", false))
	mustSet(obj, "renameSync", n.jsStub2("rename", "oldPath", "newPath", true))
	mustSet(obj, "rmdir", n.jsRmdir)
	mustSet(obj, "rmdirSync", n.jsRmdirSync)
	mustSet(obj, "stat", n.jsStat)
	mustSet(obj, "statSync", n.jsStatSync)
	mustSet(obj, "symlink", n.jsStub2("symlink", "target", "path", false))
	mustSet(obj, "symlinkSync", n.jsStub2("symlink", "target", "path", true))
	mustSet(obj, "truncate", n.jsStub1("truncate", false))
	mustSet(obj, "truncateSync", n.jsStub1("truncate", true))
	mustSet(obj, "unlink", n.jsUnlink)
	mustSet(obj, "unlinkSync", n.jsUnlinkSync)
	mustSet(obj, "unwatchFile", n.jsWatch)
	mustSet(obj, "utimes", n.jsStub1("utimes", false))
	mustSet(obj, "utimesSync", n.jsStub1("utimes", true))
	mustSet(obj, "watch", n.jsWatch)
	mustSet(obj, "watchFile", n.jsWatch)
	mustSet(obj, "writeFile", n.jsWriteFile)
	mustSet(obj, "writeFileSync", n.jsWriteFileSync)
	mustSet(obj, "exists", n.jsExists)
	mustSet(obj, "existsSync", n.jsExistsSync)
	mustSet(obj, "promises", newNodeFSPromises(iso, obj))
	return obj
}

// nodeFSPromisesBinding materializes require("fs/promises").
type nodeFSPromisesBinding struct{}

var _ Binding = nodeFSPromisesBinding{}

func (nodeFSPromisesBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	fs, err := (nodeFSBinding{}).Materialize(iso)
	if err != nil {
		return nil, err
	}
	v := fs.Get("promises")
	o, ok := v.(*goja.Object)
	if !ok {
		return iso.vm.NewObject(), nil
	}
	return o, nil
}

func newNodeFSPromises(iso *Isolate, fs *goja.Object) *goja.Object {
	v, err := runNamedScript(iso.vm, "node:fs/promises", "("+nodeFSPromisesSource+")")
	if err != nil {
		panic("goja fs/promises: " + err.Error())
	}
	fn, ok := goja.AssertFunction(v)
	if !ok {
		return iso.vm.NewObject()
	}
	out, err := fn(goja.Undefined(), fs)
	if err != nil {
		panic(err)
	}
	o, ok := out.(*goja.Object)
	if !ok {
		return iso.vm.NewObject()
	}
	return o
}

// nodeFSPromisesSource wraps callback fs methods as Promises.
const nodeFSPromisesSource = `
function (fs) {
  function wrap(name) {
    return function () {
      var args = [];
      for (var i = 0; i < arguments.length; i++) {
        args.push(arguments[i]);
      }
      var fn = fs[name];
      return new Promise(function (resolve, reject) {
        args.push(function (err, val) {
          if (err) reject(err);
          else resolve(val);
        });
        fn.apply(fs, args);
      });
    };
  }
  var names = [
    "access", "appendFile", "close", "copyFile", "lstat", "mkdir", "open",
    "readFile", "readdir", "readlink", "realpath", "rmdir", "stat",
    "unlink", "writeFile"
  ];
  var p = {};
  for (var i = 0; i < names.length; i++) {
    if (typeof fs[names[i]] === "function") {
      p[names[i]] = wrap(names[i]);
    }
  }
  p.constants = fs.constants;
  return p;
}
`

func nodeFSConstants(vm *goja.Runtime) *goja.Object {
	c := vm.NewObject()
	c.SetPrototype(nil)
	vals := map[string]int{
		"F_OK": 0, "R_OK": 4, "W_OK": 2, "X_OK": 1,
		"O_RDONLY": 0, "O_WRONLY": 1, "O_RDWR": 2,
		"O_CREAT": 64, "O_EXCL": 128, "O_NOCTTY": 256,
		"O_TRUNC": 512, "O_APPEND": 1024, "O_NONBLOCK": 2048,
		"O_DSYNC": 4096, "O_SYNC": 1052672, "O_DIRECT": 16384,
		"O_DIRECTORY": 65536, "O_NOFOLLOW": 131072, "O_NOATIME": 262144,
		"S_IFMT": 61440, "S_IFREG": 32768, "S_IFDIR": 16384,
		"S_IFCHR": 8192, "S_IFBLK": 24576, "S_IFIFO": 4096,
		"S_IFLNK": 40960, "S_IFSOCK": 49152,
		"S_IRUSR": 256, "S_IWUSR": 128, "S_IXUSR": 64,
		"S_IRWXU": 448, "S_IRGRP": 32, "S_IWGRP": 16, "S_IXGRP": 8,
		"S_IRWXG": 56, "S_IROTH": 4, "S_IWOTH": 2, "S_IXOTH": 1, "S_IRWXO": 7,
		"COPYFILE_EXCL": 1, "COPYFILE_FICLONE": 2, "COPYFILE_FICLONE_FORCE": 4,
	}
	for k, v := range vals {
		mustSet(c, k, v)
	}
	return c
}

func (n *nodeFS) jsReadFileSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	enc, _ := n.optsAndCB(call, 1)
	data, err := n.readFile(p)
	if err != nil {
		n.throwMapped("open", p, err)
	}
	return n.encode(data, enc)
}

func (n *nodeFS) jsReadFile(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	enc, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	data, err := n.readFile(p)
	if err != nil {
		n.nextTick(cb, n.sysMapped("open", p, err), goja.Undefined())
		return goja.Undefined()
	}
	n.nextTick(cb, goja.Null(), n.encode(data, enc))
	return goja.Undefined()
}

func (n *nodeFS) jsStatSync(call goja.FunctionCall) goja.Value {
	return n.statSyncCall(call, "stat")
}

func (n *nodeFS) jsLstatSync(call goja.FunctionCall) goja.Value {
	return n.statSyncCall(call, "lstat")
}

func (n *nodeFS) statSyncCall(call goja.FunctionCall, op string) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	info, err := n.statPath(p)
	if err != nil {
		if !throwIfNoEntry(call.Argument(1)) {
			return goja.Undefined()
		}
		n.throwMapped(op, p, err)
	}
	return n.statObj(info)
}

func (n *nodeFS) jsStat(call goja.FunctionCall) goja.Value {
	return n.statCall(call, "stat")
}

func (n *nodeFS) jsLstat(call goja.FunctionCall) goja.Value {
	return n.statCall(call, "lstat")
}

func (n *nodeFS) statCall(call goja.FunctionCall, op string) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	info, err := n.statPath(p)
	if err != nil {
		n.nextTick(cb, n.sysMapped(op, p, err), goja.Undefined())
		return goja.Undefined()
	}
	n.nextTick(cb, goja.Null(), n.statObj(info))
	return goja.Undefined()
}

func (n *nodeFS) jsExists(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "cb" argument must be of type function. Received undefined`)
	}
	cb, ok := goja.AssertFunction(call.Argument(1))
	if !ok {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "cb" argument must be of type function`)
	}
	n.nextTick(cb, n.iso.vm.ToValue(n.pathExists(call.Argument(0))))
	return goja.Undefined()
}

func (n *nodeFS) jsExistsSync(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return n.iso.vm.ToValue(false)
	}
	return n.iso.vm.ToValue(n.pathExists(call.Argument(0)))
}

func (n *nodeFS) pathExists(v goja.Value) bool {
	p, err := n.parsePath(v)
	if err != nil {
		return false
	}
	_, err = n.statPath(p)
	return err == nil
}

func (n *nodeFS) jsAccessSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	if _, err := n.statPath(p); err != nil {
		n.throwMapped("access", p, err)
	}
	return goja.Undefined()
}

func (n *nodeFS) jsAccess(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	if _, err := n.statPath(p); err != nil {
		n.nextTick(cb, n.sysMapped("access", p, err))
		return goja.Undefined()
	}
	n.nextTick(cb, goja.Null())
	return goja.Undefined()
}

func (n *nodeFS) jsReaddirSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	ents, err := n.readDir(p)
	if err != nil {
		n.throwMapped("scandir", p, err)
	}
	return n.readdirResult(ents, call.Argument(1))
}

func (n *nodeFS) jsReaddir(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	ents, err := n.readDir(p)
	if err != nil {
		n.nextTick(cb, n.sysMapped("scandir", p, err), goja.Undefined())
		return goja.Undefined()
	}
	n.nextTick(cb, goja.Null(), n.readdirResult(ents, call.Argument(1)))
	return goja.Undefined()
}

func (n *nodeFS) readdirResult(ents []fs.DirEntry, opts goja.Value) goja.Value {
	if withFileTypes(opts) {
		return n.dirents(ents)
	}
	return n.dirNames(ents)
}

func (n *nodeFS) dirNames(ents []fs.DirEntry) goja.Value {
	names := make([]any, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return n.iso.vm.ToValue(names)
}

func (n *nodeFS) dirents(ents []fs.DirEntry) goja.Value {
	out := make([]any, 0, len(ents))
	for _, e := range ents {
		out = append(out, n.direntObj(e))
	}
	return n.iso.vm.ToValue(out)
}

func (n *nodeFS) direntObj(e fs.DirEntry) *goja.Object {
	o := n.iso.vm.NewObject()
	mode := e.Type()
	mustSet(o, "name", e.Name())
	mustSet(o, "isFile", n.statFlag(mode.IsRegular()))
	mustSet(o, "isDirectory", n.statFlag(e.IsDir()))
	mustSet(o, "isSymbolicLink", n.statFlag(mode&fs.ModeSymlink != 0))
	mustSet(o, "isBlockDevice", n.statFlag(false))
	mustSet(o, "isCharacterDevice", n.statFlag(false))
	mustSet(o, "isFIFO", n.statFlag(false))
	mustSet(o, "isSocket", n.statFlag(false))
	return o
}

func withFileTypes(v goja.Value) bool {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return false
	}
	flag := obj.Get("withFileTypes")
	if flag == nil || goja.IsUndefined(flag) {
		return false
	}
	return flag.ToBoolean()
}

func (n *nodeFS) jsRealpathSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	out, err := n.realpath(p)
	if err != nil {
		n.throwMapped("lstat", p, err)
	}
	return n.iso.vm.ToValue(out)
}

func (n *nodeFS) jsRealpath(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	out, err := n.realpath(p)
	if err != nil {
		n.nextTick(cb, n.sysMapped("lstat", p, err), goja.Undefined())
		return goja.Undefined()
	}
	n.nextTick(cb, goja.Null(), n.iso.vm.ToValue(out))
	return goja.Undefined()
}

func (n *nodeFS) realpath(p string) (string, error) {
	if _, err := n.statPath(p); err != nil {
		return "", err
	}
	if rp, ok := n.fsys.(RealpathFS); ok {
		real, err := rp.Realpath(p)
		if err != nil {
			return "", err
		}
		p = real
	}
	return n.publicPath(p), nil
}

func (n *nodeFS) setNative(obj *goja.Object, name string, fn func(goja.FunctionCall) goja.Value) {
	v := n.iso.vm.ToValue(fn)
	mustSet(obj, name, v)
	if o, ok := v.(*goja.Object); ok {
		mustSet(o, "native", v)
	}
}

func (n *nodeFS) jsOpenSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	if _, err := n.statPath(p); err != nil {
		n.throwMapped("open", p, err)
	}
	return n.iso.vm.ToValue(3)
}

func (n *nodeFS) jsOpen(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	if _, err := n.statPath(p); err != nil {
		n.nextTick(cb, n.sysMapped("open", p, err), goja.Undefined())
		return goja.Undefined()
	}
	n.nextTick(cb, goja.Null(), n.iso.vm.ToValue(3))
	return goja.Undefined()
}

func (n *nodeFS) jsCloseSync(call goja.FunctionCall) goja.Value {
	return goja.Undefined()
}

func (n *nodeFS) jsClose(call goja.FunctionCall) goja.Value {
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	n.nextTick(cb, goja.Null())
	return goja.Undefined()
}

func (n *nodeFS) jsReadlinkSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	n.throwReadlink(p)
	return goja.Undefined()
}

func (n *nodeFS) jsReadlink(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	if _, err := n.statPath(p); err != nil {
		n.nextTick(cb, n.sysMapped("readlink", p, err), goja.Undefined())
		return goja.Undefined()
	}
	n.nextTick(cb, n.sysObj("EINVAL", -22, "readlink", p), goja.Undefined())
	return goja.Undefined()
}

func (n *nodeFS) throwReadlink(p string) {
	if _, err := n.statPath(p); err != nil {
		n.throwMapped("readlink", p, err)
	}
	n.throwSys("EINVAL", -22, "readlink", p)
}

func (n *nodeFS) jsWriteFileSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	n.writeFile(p, valueBytes(call.Argument(1)), true, nil)
	return goja.Undefined()
}

func (n *nodeFS) jsWriteFile(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 2)
	if cb == nil {
		if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
			cb = fn
		}
	}
	n.requireCB(cb)
	n.writeFile(p, valueBytes(call.Argument(1)), false, cb)
	return goja.Undefined()
}

func (n *nodeFS) writeFile(p string, data []byte, sync bool, cb goja.Callable) {
	w := n.writer()
	if w == nil {
		n.fail("open", p, errReadOnly, sync, cb, false)
		return
	}
	if err := w.WriteFile(p, data, 0o666); err != nil {
		n.fail("open", p, err, sync, cb, false)
		return
	}
	if !sync {
		n.nextTick(cb, goja.Null())
	}
}

func (n *nodeFS) jsAppendFileSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	n.appendFile(p, valueBytes(call.Argument(1)), true, nil)
	return goja.Undefined()
}

func (n *nodeFS) jsAppendFile(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 2)
	if cb == nil {
		if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
			cb = fn
		}
	}
	n.requireCB(cb)
	n.appendFile(p, valueBytes(call.Argument(1)), false, cb)
	return goja.Undefined()
}

func (n *nodeFS) appendFile(p string, extra []byte, sync bool, cb goja.Callable) {
	w := n.writer()
	if w == nil {
		n.fail("open", p, errReadOnly, sync, cb, false)
		return
	}
	cur, err := n.readFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		n.fail("open", p, err, sync, cb, false)
		return
	}
	if err := w.WriteFile(p, append(cur, extra...), 0o666); err != nil {
		n.fail("open", p, err, sync, cb, false)
		return
	}
	if !sync {
		n.nextTick(cb, goja.Null())
	}
}

func (n *nodeFS) jsMkdirSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	n.mkdir(p, true, nil)
	return goja.Undefined()
}

func (n *nodeFS) jsMkdir(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	n.mkdir(p, false, cb)
	return goja.Undefined()
}

func (n *nodeFS) mkdir(p string, sync bool, cb goja.Callable) {
	w := n.writer()
	if w == nil {
		n.fail("mkdir", p, errReadOnly, sync, cb, false)
		return
	}
	if err := w.Mkdir(p, 0o777); err != nil {
		n.fail("mkdir", p, err, sync, cb, false)
		return
	}
	if !sync {
		n.nextTick(cb, goja.Null())
	}
}

func (n *nodeFS) jsUnlinkSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	n.remove(p, "unlink", true, nil)
	return goja.Undefined()
}

func (n *nodeFS) jsUnlink(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	n.remove(p, "unlink", false, cb)
	return goja.Undefined()
}

func (n *nodeFS) jsRmdirSync(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	n.remove(p, "rmdir", true, nil)
	return goja.Undefined()
}

func (n *nodeFS) jsRmdir(call goja.FunctionCall) goja.Value {
	p := n.mustPath(call.Argument(0), "path")
	_, cb := n.optsAndCB(call, 1)
	n.requireCB(cb)
	n.remove(p, "rmdir", false, cb)
	return goja.Undefined()
}

func (n *nodeFS) remove(p, op string, sync bool, cb goja.Callable) {
	if _, err := n.statPath(p); err != nil {
		n.fail(op, p, err, sync, cb, false)
		return
	}
	w := n.writer()
	if w == nil {
		n.fail(op, p, errReadOnly, sync, cb, false)
		return
	}
	if err := w.Remove(p); err != nil {
		n.fail(op, p, err, sync, cb, false)
		return
	}
	if !sync {
		n.nextTick(cb, goja.Null())
	}
}

func (n *nodeFS) jsCopyFileSync(call goja.FunctionCall) goja.Value {
	src, dest := n.mustTwo(call, "src", "dest")
	n.copyFile(src, dest, true, nil)
	return goja.Undefined()
}

func (n *nodeFS) jsCopyFile(call goja.FunctionCall) goja.Value {
	src, dest := n.mustTwo(call, "src", "dest")
	_, cb := n.optsAndCB(call, 2)
	n.requireCB(cb)
	n.copyFile(src, dest, false, cb)
	return goja.Undefined()
}

func (n *nodeFS) copyFile(src, dest string, sync bool, cb goja.Callable) {
	data, err := n.readFile(src)
	if err != nil {
		n.fail("copyfile", src, err, sync, cb, false)
		return
	}
	w := n.writer()
	if w == nil {
		n.fail("copyfile", dest, errReadOnly, sync, cb, false)
		return
	}
	if err := w.WriteFile(dest, data, 0o666); err != nil {
		n.fail("copyfile", dest, err, sync, cb, false)
		return
	}
	if !sync {
		n.nextTick(cb, goja.Null())
	}
}

func (n *nodeFS) jsWatch(call goja.FunctionCall) goja.Value {
	n.mustPath(call.Argument(0), "filename")
	return goja.Undefined()
}

func (n *nodeFS) jsStub1(op string, sync bool) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		p := n.mustPath(call.Argument(0), "path")
		if sync {
			n.fail(op, p, errReadOnly, true, nil, true)
			return goja.Undefined()
		}
		_, cb := n.optsAndCB(call, 1)
		n.requireCB(cb)
		n.fail(op, p, errReadOnly, false, cb, false)
		return goja.Undefined()
	}
}

func (n *nodeFS) jsStub2(op, n1, n2 string, sync bool) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		_, dest := n.mustTwo(call, n1, n2)
		if sync {
			n.fail(op, dest, errReadOnly, true, nil, true)
			return goja.Undefined()
		}
		_, cb := n.optsAndCB(call, 2)
		n.requireCB(cb)
		n.fail(op, dest, errReadOnly, false, cb, false)
		return goja.Undefined()
	}
}

func (n *nodeFS) fail(op, p string, err error, sync bool, cb goja.Callable, mustExist bool) {
	if mustExist {
		if _, e := n.statPath(p); e != nil {
			n.throwMapped(op, p, e)
		}
	}
	if sync {
		n.throwMapped(op, p, err)
	}
	n.nextTick(cb, n.sysMapped(op, p, err))
}

func (n *nodeFS) writer() WriteFS {
	if w, ok := n.fsys.(WriteFS); ok {
		return w
	}
	return nil
}

func (n *nodeFS) readFile(name string) ([]byte, error) {
	info, err := n.statPath(name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errIsDir
	}
	if n.fsys == nil {
		return nil, fs.ErrNotExist
	}
	if rf, ok := n.fsys.(fs.ReadFileFS); ok {
		return rf.ReadFile(name)
	}
	f, err := n.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (n *nodeFS) statPath(name string) (fs.FileInfo, error) {
	if n.fsys == nil {
		return nil, fs.ErrNotExist
	}
	if sf, ok := n.fsys.(fs.StatFS); ok {
		return sf.Stat(name)
	}
	f, err := n.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (n *nodeFS) readDir(name string) ([]fs.DirEntry, error) {
	if n.fsys == nil {
		return nil, fs.ErrNotExist
	}
	if info, err := n.statPath(name); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, errNotDir
	}
	if rd, ok := n.fsys.(fs.ReadDirFS); ok {
		return rd.ReadDir(name)
	}
	f, err := n.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d, ok := f.(fs.ReadDirFile)
	if !ok {
		return nil, errNotDir
	}
	return d.ReadDir(-1)
}

func (n *nodeFS) encode(data []byte, enc string) goja.Value {
	if isUTF8Enc(enc) {
		return n.iso.vm.ToValue(string(data))
	}
	cp := append([]byte(nil), data...)
	ab := n.iso.vm.NewArrayBuffer(cp)
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Uint8Array"))
	if !ok {
		return n.iso.vm.ToValue(ab)
	}
	v, err := ctor(nil, n.iso.vm.ToValue(ab))
	if err != nil {
		panic(err)
	}
	// Node returns Buffer; Vite does readFileSync(url).toString().
	mustSet(v, "toString", func(goja.FunctionCall) string { return string(cp) })
	return v
}

func (n *nodeFS) statObj(info fs.FileInfo) *goja.Object {
	o := n.iso.vm.NewObject()
	isFile := info.Mode().IsRegular()
	isDir := info.IsDir()
	isLink := info.Mode()&fs.ModeSymlink != 0
	mustSet(o, "size", info.Size())
	mustSet(o, "mode", int64(info.Mode()))
	mustSet(o, "isFile", n.statFlag(isFile))
	mustSet(o, "isDirectory", n.statFlag(isDir))
	mustSet(o, "isSymbolicLink", n.statFlag(isLink))
	mustSet(o, "isBlockDevice", n.statFlag(false))
	mustSet(o, "isCharacterDevice", n.statFlag(false))
	mustSet(o, "isFIFO", n.statFlag(false))
	mustSet(o, "isSocket", n.statFlag(false))
	mustSet(o, "mtimeMs", float64(info.ModTime().UnixMilli()))
	return o
}

func (n *nodeFS) statFlag(v bool) func(goja.FunctionCall) goja.Value {
	return func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue(v)
	}
}

func (n *nodeFS) mustTwo(call goja.FunctionCall, n1, n2 string) (string, string) {
	return n.mustPath(call.Argument(0), n1), n.mustPath(call.Argument(1), n2)
}

func (n *nodeFS) mustPath(v goja.Value, name string) string {
	p, err := n.parsePath(v)
	if errors.Is(err, errPathNUL) {
		n.throwType("ERR_INVALID_ARG_VALUE", "The argument '"+name+"' is invalid. Received path containing NUL")
	}
	if errors.Is(err, errBadPathType) {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "`+name+`" argument must be of type string or an instance of Buffer or URL`)
	}
	if err != nil {
		return ".."
	}
	return p
}

func (n *nodeFS) parsePath(v goja.Value) (string, error) {
	arg := inspectPathArg(v)
	if !arg.ok {
		return "", errBadPathType
	}
	raw := arg.raw
	if arg.fileURL {
		if fileURLHasNUL(raw) {
			return "", errPathNUL
		}
		raw = fileURLToGuest(raw)
	} else if strings.ContainsRune(raw, 0) {
		return "", errPathNUL
	}
	cwd := ""
	if n != nil && n.iso != nil {
		cwd = n.iso.cwd
	}
	return normalizeGuest(stripCwdPrefix(raw, cwd))
}

// stripCwdPrefix maps a host-absolute path under process.cwd() into the
// mounted tree. Vite resolves against cwd; normalizeGuest would otherwise
// treat "/home/.../proj/src" as guest "home/.../proj/src".
func stripCwdPrefix(p, cwd string) string {
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	cwd = path.Clean(strings.ReplaceAll(cwd, "\\", "/"))
	if cwd == "." || cwd == "" {
		return p
	}
	alts := []string{cwd}
	if trimmed := strings.TrimPrefix(cwd, "/"); trimmed != cwd && trimmed != "" {
		alts = append(alts, trimmed)
	}
	for _, c := range alts {
		if p == c {
			return "."
		}
		if strings.HasPrefix(p, c+"/") {
			rest := p[len(c)+1:]
			if rest == "" {
				return "."
			}
			return rest
		}
	}
	return p
}

// publicPath is the Node-facing absolute path for a guest tree path.
func (n *nodeFS) publicPath(guest string) string {
	cwd := "/"
	if n != nil && n.iso != nil {
		cwd = strings.ReplaceAll(n.iso.cwd, "\\", "/")
		cwd = strings.TrimRight(cwd, "/")
		if cwd == "" || cwd == "." {
			cwd = "/"
		}
	}
	if guest == "" || guest == "." {
		if strings.HasPrefix(cwd, "/") {
			return cwd
		}
		return "/"
	}
	if !strings.HasPrefix(cwd, "/") {
		return "/" + guest
	}
	if cwd == "/" {
		return "/" + guest
	}
	return cwd + "/" + guest
}

type pathArg struct {
	raw     string
	fileURL bool
	ok      bool
}

func inspectPathArg(v goja.Value) pathArg {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return pathArg{}
	}
	switch x := v.Export().(type) {
	case string:
		return pathArg{raw: x, ok: true}
	case []byte:
		return pathArg{raw: string(x), ok: true}
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return pathArg{}
	}
	href := obj.Get("href")
	if href == nil || goja.IsUndefined(href) || goja.IsNull(href) {
		return pathArg{}
	}
	h := href.String()
	if isFileHref(h) {
		return pathArg{raw: h, fileURL: true, ok: true}
	}
	proto := obj.Get("protocol")
	if proto != nil && !goja.IsUndefined(proto) && proto.String() != "" {
		return pathArg{}
	}
	return pathArg{}
}

func isFileHref(h string) bool {
	return len(h) >= 5 && strings.EqualFold(h[:5], "file:")
}

func fileURLHasNUL(href string) bool {
	if strings.ContainsRune(href, 0) {
		return true
	}
	return strings.Contains(strings.ToLower(href), "%00")
}

func fileURLToGuest(href string) string {
	s := href
	if len(s) >= 5 && strings.EqualFold(s[:5], "file:") {
		s = s[5:]
	}
	s = strings.TrimPrefix(s, "//")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func normalizeGuest(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimLeft(p, "/")
	if p == "" {
		return ".", nil
	}
	cleaned := path.Clean(p)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errPathEscape
	}
	if !fs.ValidPath(cleaned) {
		return "", errPathEscape
	}
	return cleaned, nil
}

func (n *nodeFS) optsAndCB(call goja.FunctionCall, start int) (enc string, cb goja.Callable) {
	if start >= len(call.Arguments) {
		return "", nil
	}
	a := call.Argument(start)
	if fn, ok := goja.AssertFunction(a); ok {
		return "", fn
	}
	enc = encodingOf(a)
	if start+1 < len(call.Arguments) {
		if fn, ok := goja.AssertFunction(call.Argument(start + 1)); ok {
			return enc, fn
		}
	}
	return enc, nil
}

func throwIfNoEntry(v goja.Value) bool {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return true
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return true
	}
	flag := obj.Get("throwIfNoEntry")
	if flag == nil || goja.IsUndefined(flag) {
		return true
	}
	return flag.ToBoolean()
}

func encodingOf(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	if obj, ok := v.(*goja.Object); ok {
		if _, isFn := goja.AssertFunction(v); isFn {
			return ""
		}
		if enc := obj.Get("encoding"); enc != nil && !goja.IsUndefined(enc) && !goja.IsNull(enc) {
			if _, ok := enc.Export().(string); ok || enc.ToBoolean() {
				return enc.String()
			}
		}
		return ""
	}
	if _, ok := v.Export().(string); ok {
		return v.String()
	}
	return ""
}

func isUTF8Enc(enc string) bool {
	e := strings.ToLower(enc)
	e = strings.ReplaceAll(e, "-", "")
	e = strings.ReplaceAll(e, "_", "")
	return e == "utf8"
}

func valueBytes(v goja.Value) []byte {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	switch x := v.Export().(type) {
	case string:
		return []byte(x)
	case []byte:
		return append([]byte(nil), x...)
	}
	return []byte(v.String())
}

func (n *nodeFS) requireCB(cb goja.Callable) {
	if cb == nil {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "cb" argument must be of type function. Received undefined`)
	}
}

func (n *nodeFS) nextTick(fn goja.Callable, args ...goja.Value) {
	n.iso.timers.schedule(fn, args, 0, 0, n.iso.now())
}

func (n *nodeFS) throwType(code, msg string) {
	e := n.iso.vm.NewTypeError(msg)
	_ = e.Set("code", code)
	panic(e)
}

func (n *nodeFS) throwMapped(op, p string, err error) {
	panic(n.sysMapped(op, p, err))
}

func (n *nodeFS) throwSys(code string, errno int, op, p string) {
	panic(n.sysObj(code, errno, op, p))
}

func (n *nodeFS) sysMapped(op, p string, err error) *goja.Object {
	code, errno := mapFSErr(err)
	return n.sysObj(code, errno, op, p)
}

func (n *nodeFS) sysObj(code string, errno int, op, p string) *goja.Object {
	msg := fmt.Sprintf("%s: %s, %s '%s'", code, errnoText(code), op, p)
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Error"))
	if !ok {
		e := n.iso.vm.NewGoError(errors.New(msg))
		_ = e.Set("code", code)
		return e
	}
	o, err := ctor(nil, n.iso.vm.ToValue(msg))
	if err != nil {
		panic(err)
	}
	_ = o.Set("code", code)
	_ = o.Set("errno", errno)
	_ = o.Set("syscall", op)
	_ = o.Set("path", p)
	return o
}

func mapFSErr(err error) (string, int) {
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, errPathEscape):
		return "ENOENT", -2
	case errors.Is(err, errReadOnly):
		return "EROFS", -30
	case errors.Is(err, fs.ErrPermission):
		return "EPERM", -1
	case errors.Is(err, errIsDir):
		return "EISDIR", -21
	case errors.Is(err, errNotDir):
		return "ENOTDIR", -20
	case errors.Is(err, fs.ErrExist):
		return "EEXIST", -17
	case errors.Is(err, fs.ErrInvalid):
		return "EINVAL", -22
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Err != err {
		return mapFSErr(pe.Err)
	}
	return "EIO", -5
}

func errnoText(code string) string {
	switch code {
	case "ENOENT":
		return "no such file or directory"
	case "EPERM":
		return "operation not permitted"
	case "EACCES":
		return "permission denied"
	case "EEXIST":
		return "file already exists"
	case "ENOTDIR":
		return "not a directory"
	case "EISDIR":
		return "illegal operation on a directory"
	case "EINVAL":
		return "invalid argument"
	case "EROFS":
		return "read-only file system"
	default:
		return "i/o error"
	}
}
