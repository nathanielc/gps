package gps

import (
	"fmt"
	"sync"
)

// singleSourceCache provides a method set for storing and retrieving data about
// a single source.
type singleSourceCache interface {
	// Store the manifest and lock information for a given revision, as defined by
	// a particular ProjectAnalyzer.
	setProjectInfo(Revision, ProjectAnalyzer, projectInfo)

	// Get the manifest and lock information for a given revision, as defined by
	// a particular ProjectAnalyzer.
	getProjectInfo(Revision, ProjectAnalyzer) (projectInfo, bool)

	// Store a PackageTree for a given revision.
	setPackageTree(Revision, PackageTree)

	// Get the PackageTree for a given revision.
	getPackageTree(Revision) (PackageTree, bool)

	// Store the mappings between a set of PairedVersions' surface versions
	// their corresponding revisions.
	//
	// If flush is true, the existing list of versions will be purged before
	// writing. Revisions will have their pairings purged, but record of the
	// revision existing will be kept, on the assumption that revisions are
	// immutable and permanent.
	storeVersionMap(versionList []PairedVersion, flush bool)

	// Get the list of unpaired versions corresponding to the given revision.
	getVersionsFor(Revision) ([]UnpairedVersion, bool)

	// Gets all the version pairs currently known to the cache.
	getAllVersions() []Version
	//getAllVersions() []PairedVersion

	// Get the revision corresponding to the given unpaired version.
	getRevisionFor(UnpairedVersion) (Revision, bool)

	// Attempt to convert the given Version to a Revision, given information
	// currently present in the cache, and in the Version itself.
	toRevision(v Version) (Revision, bool)

	// Attempt to convert the given Version to an UnpairedVersion, given
	// information currently present in the cache, or in the Version itself.
	//
	// If the input is a revision and multiple UnpairedVersions are associated
	// with it, whatever happens to be the first is returned.
	toUnpaired(v Version) (UnpairedVersion, bool)
}

type singleSourceCacheMemory struct {
	mut    sync.RWMutex // protects all maps
	infos  map[ProjectAnalyzer]map[Revision]projectInfo
	ptrees map[Revision]PackageTree
	vMap   map[UnpairedVersion]Revision
	rMap   map[Revision][]UnpairedVersion
}

func newMemoryCache() singleSourceCache {
	return &singleSourceCacheMemory{
		infos:  make(map[ProjectAnalyzer]map[Revision]projectInfo),
		ptrees: make(map[Revision]PackageTree),
		vMap:   make(map[UnpairedVersion]Revision),
		rMap:   make(map[Revision][]UnpairedVersion),
	}
}

func (c *singleSourceCacheMemory) setProjectInfo(r Revision, an ProjectAnalyzer, pi projectInfo) {
	c.mut.Lock()
	inner, has := c.infos[an]
	if !has {
		inner = make(map[Revision]projectInfo)
		c.infos[an] = inner
	}
	inner[r] = pi

	// Ensure there's at least an entry in the rMap so that the rMap always has
	// a complete picture of the revisions we know to exist
	if _, has = c.rMap[r]; !has {
		c.rMap[r] = nil
	}
	c.mut.Unlock()
}

func (c *singleSourceCacheMemory) getProjectInfo(r Revision, an ProjectAnalyzer) (projectInfo, bool) {
	c.mut.Lock()
	defer c.mut.Unlock()

	inner, has := c.infos[an]
	if !has {
		return projectInfo{}, false
	}

	pi, has := inner[r]
	return pi, has
}

func (c *singleSourceCacheMemory) setPackageTree(r Revision, ptree PackageTree) {
	c.mut.Lock()
	c.ptrees[r] = ptree

	// Ensure there's at least an entry in the rMap so that the rMap always has
	// a complete picture of the revisions we know to exist
	if _, has := c.rMap[r]; !has {
		c.rMap[r] = nil
	}
	c.mut.Unlock()
}

func (c *singleSourceCacheMemory) getPackageTree(r Revision) (PackageTree, bool) {
	c.mut.Lock()
	ptree, has := c.ptrees[r]
	c.mut.Unlock()
	return ptree, has
}

func (c *singleSourceCacheMemory) storeVersionMap(versionList []PairedVersion, flush bool) {
	c.mut.Lock()
	if flush {
		// TODO(sdboyer) how do we handle cache consistency here - revs that may
		// be out of date vis-a-vis the ptrees or infos maps?
		for r := range c.rMap {
			c.rMap[r] = nil
		}

		c.vMap = make(map[UnpairedVersion]Revision)
	}

	for _, v := range versionList {
		pv := v.(PairedVersion)
		u, r := pv.Unpair(), pv.Underlying()
		c.vMap[u] = r
		c.rMap[r] = append(c.rMap[r], u)
	}
	c.mut.Unlock()
}

func (c *singleSourceCacheMemory) getVersionsFor(r Revision) ([]UnpairedVersion, bool) {
	c.mut.Lock()
	versionList, has := c.rMap[r]
	c.mut.Unlock()
	return versionList, has
}

//func (c *singleSourceCacheMemory) getAllVersions() []PairedVersion {
func (c *singleSourceCacheMemory) getAllVersions() []Version {
	//vlist := make([]PairedVersion, 0, len(c.vMap))
	vlist := make([]Version, 0, len(c.vMap))
	for v, r := range c.vMap {
		vlist = append(vlist, v.Is(r))
	}
	return vlist
}

func (c *singleSourceCacheMemory) getRevisionFor(uv UnpairedVersion) (Revision, bool) {
	c.mut.Lock()
	r, has := c.vMap[uv]
	c.mut.Unlock()
	return r, has
}

func (c *singleSourceCacheMemory) toRevision(v Version) (Revision, bool) {
	switch t := v.(type) {
	case Revision:
		return t, true
	case PairedVersion:
		return t.Underlying(), true
	case UnpairedVersion:
		c.mut.Lock()
		r, has := c.vMap[t]
		c.mut.Unlock()
		return r, has
	default:
		panic(fmt.Sprintf("Unknown version type %T", v))
	}
}

func (c *singleSourceCacheMemory) toUnpaired(v Version) (UnpairedVersion, bool) {
	switch t := v.(type) {
	case UnpairedVersion:
		return t, true
	case PairedVersion:
		return t.Unpair(), true
	case Revision:
		c.mut.Lock()
		upv, has := c.rMap[t]
		c.mut.Unlock()

		if has && len(upv) > 0 {
			return upv[0], true
		}
		return nil, false
	default:
		panic(fmt.Sprintf("unknown version type %T", v))
	}
}