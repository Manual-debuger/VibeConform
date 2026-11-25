package contract

import (
	"errors"
	"io/fs"
)

// "Has dependencies" decides D-01's no-lockfile case (spec 0053, D-01).

// errNoManifest is a component without its language's manifest. V-04's
// component checks report that; D-01 has nothing to decide.
var errNoManifest = errors.New("no manifest")

// hasDependencies reports whether a component's manifest declares
// dependencies, and names the manifest. The language record's reader
// decides what counts as a dependency.
func hasDependencies(env *Env, comp Component) (manifest string, deps bool, err error) {
	lang := comp.language()
	manifest = joinRel(comp.Path, lang.manifest)
	data, err := env.ReadFile(manifest)
	if errors.Is(err, fs.ErrNotExist) {
		return manifest, false, errNoManifest
	}
	if err != nil {
		return manifest, false, err
	}
	deps, err = lang.hasDependencies(manifest, data)
	return manifest, deps, err
}
