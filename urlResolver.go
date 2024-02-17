/*
 * (C) Copyright 2024 Johan Michel PIQUET, France (https://johanpiquet.fr/).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package httpServer

import (
	"sort"
	"strconv"
	"strings"
)

// UrlResolver allows to bind an listener to an url part. It's a router.
type UrlResolver struct {
	root urlResolverPathPart
}

type UrlResolverResult struct {
	Segments          []string
	RemainingSegments []string
	Target            any

	Middlewares []any

	wildcards    []string
	rawWildcards []string
}

type UrlResolverTreeItem struct {
	Path    string
	Handler any
	Tag     any
}

type urlResolverPathWildCard struct {
	prefix string
	next   *urlResolverPathPart
}

type urlResolverPathPart struct {
	parent *urlResolverPathPart

	segmentMap        map[string]*urlResolverPathPart
	beginByMap        map[string]*urlResolverPathPart
	beginByMapOrdered []urlResolverPathWildCard

	pathPrefix string

	exactHandler    any
	exactHandlerTag any

	catchAllHandler    any
	catchAllHandlerTag any

	// exactMiddlewares are the middleware applied when the url match exactly this node.
	// It will be merged with each childMiddlewares from the parents.
	exactMiddlewares []any

	// exactMiddlewaresCache is the merge between exactMiddlewares and childMiddlewares from his parents.
	exactMiddlewaresCache []any

	// exactMiddlewaresCache is the merge between childMiddlewares and the childMiddlewares from his parents.
	catchAllMiddlewaresCache []any

	// childMiddlewares are the middlewares applied to the children when one of them if matching the url.
	childMiddlewares []any
}

func NewUrlResolver() *UrlResolver {
	return &UrlResolver{}
}

func (m *UrlResolverResult) GetWildcards() []string {
	if m.rawWildcards == nil {
		return nil
	}

	if m.wildcards != nil {
		return m.wildcards
	}

	var dst []string
	src := m.rawWildcards
	size := len(m.rawWildcards)

	for i := size - 1; i >= 0; i-- {
		dst = append(dst, src[i])
	}

	m.wildcards = dst

	return dst
}

func (m *UrlResolver) Print() {
	m.root.print("")
}

func (m *UrlResolver) Find(path string) UrlResolverResult {
	result := UrlResolverResult{}

	var parts []string

	if path == "/" {
		parts = []string{}
	} else {
		parts = (strings.Split(path, "/"))[1:]
	}

	result.Segments = parts
	m.root.find(parts, &result)

	return result
}

func (m *UrlResolver) Add(path string, handler any, tag any) {
	if len(path) != 0 {
		if path[0] == '/' {
			path = path[1:]
		}
	}

	parts := strings.Split(path, "/")
	if path == "" {
		parts = parts[0:0]
	}

	m.root.addPath(parts, "", handler, tag)
}

// AppendMiddleware add a handler which is always executed before the other handlers.
func (m *UrlResolver) AppendMiddleware(path string, handler any, tag any) {
	if len(path) != 0 {
		if path[0] == '/' {
			path = path[1:]
		}
	}

	isExactMatch := !strings.HasSuffix(path, "/*")

	if !isExactMatch {
		path = path[0 : len(path)-2]
	}

	parts := strings.Split(path, "/")
	if path == "" {
		parts = parts[0:0]
	}

	m.root.appendMiddleware(parts, "", handler, tag, isExactMatch)
}

func (m *UrlResolver) DumpTree() []UrlResolverTreeItem {
	return m.root.dumpTree(nil)
}

func (m *urlResolverPathPart) dumpTree(tree []UrlResolverTreeItem) []UrlResolverTreeItem {
	if m.exactHandler != nil {
		prefix := m.pathPrefix
		if prefix == "" {
			prefix = "/"
		}

		tree = append(tree, UrlResolverTreeItem{Path: prefix, Handler: m.exactHandler, Tag: m.exactHandlerTag})
	}

	if m.catchAllHandler != nil {
		tree = append(tree, UrlResolverTreeItem{Path: m.pathPrefix + "/*", Handler: m.catchAllHandler, Tag: m.catchAllHandlerTag})
	}

	if m.exactMiddlewares != nil {
		prefix := m.pathPrefix
		if prefix == "" {
			prefix = "/"
		}

		for _, entry := range m.exactMiddlewares {
			tree = append(tree, UrlResolverTreeItem{Path: "@" + prefix, Handler: entry, Tag: m.exactHandlerTag})
		}
	}

	if m.childMiddlewares != nil {
		prefix := m.pathPrefix
		if prefix == "" {
			prefix = "/"
		}

		for _, entry := range m.childMiddlewares {
			tree = append(tree, UrlResolverTreeItem{Path: "@" + prefix + "/*", Handler: entry, Tag: m.exactHandlerTag})
		}
	}

	if m.beginByMap != nil {
		for _, entry := range m.beginByMap {
			tree = entry.dumpTree(tree)
		}
	}

	if m.segmentMap != nil {
		for _, entry := range m.segmentMap {
			tree = entry.dumpTree(tree)
		}
	}

	return tree
}

func (m *urlResolverPathPart) walk(handler func(*urlResolverPathPart)) {
	handler(m)

	if m.beginByMap != nil {
		for _, entry := range m.beginByMap {
			entry.walk(handler)
		}
	}

	if m.segmentMap != nil {
		for _, entry := range m.segmentMap {
			entry.walk(handler)
		}
	}
}

func (m *urlResolverPathPart) print(tab string) {
	info := ""

	if m.exactHandler != nil {
		info += "[exactHandler]"
	}

	if m.catchAllHandler != nil {
		info += "[catchAllHandler]"
	}

	if m.exactMiddlewares != nil {
		info += "[exactMiddlewares (" + strconv.Itoa(len(m.exactMiddlewares)) + ")]"
	}

	if m.childMiddlewares != nil {
		info += "[childMiddlewares (" + strconv.Itoa(len(m.childMiddlewares)) + ")]"
	}

	if m.beginByMap != nil {
		for key := range m.beginByMap {
			info += "[beginBy:" + key + "]"
		}
	}

	if info == "" {
		info = "[empty]"
	}

	url := m.pathPrefix
	if url == "" {
		url = "/"
	}

	println(spaceRight(50, tab+"|- "+url), info)

	if m.beginByMap != nil {
		for _, value := range m.beginByMap {
			value.print(tab + "   ")
		}
	}

	if m.segmentMap != nil {
		for _, value := range m.segmentMap {
			value.print(tab + "   ")
		}
	}
}

func (m *urlResolverPathPart) find(segments []string, result *UrlResolverResult) bool {
	// Exact same length ?
	//
	if len(segments) == 0 {
		if m.exactHandler == nil {
			return false
		}

		result.Target = m.exactHandler
		result.RemainingSegments = segments

		if m.exactMiddlewares != nil {
			result.Middlewares = m.exactMiddlewaresCache
		}

		return true
	}

	s0 := segments[0]

	// Exist in a sub-path?
	//
	if m.segmentMap != nil {
		entry := m.segmentMap[s0]

		if entry != nil {
			if entry.find(segments[1:], result) {
				return true
			}
		}
	}

	// Starts with a prefix?
	//
	if m.beginByMap != nil {
		if m.beginByMapOrdered == nil {
			var entries []urlResolverPathWildCard

			for key, entry := range m.beginByMap {
				entries = append(entries, urlResolverPathWildCard{prefix: key, next: entry})
			}

			// Sort from taller to shorter.
			//
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].prefix > entries[j].prefix
			})

			m.beginByMapOrdered = entries
		}

		for _, entry := range m.beginByMapOrdered {
			if strings.HasPrefix(s0, entry.prefix) && (entry.prefix != s0) {
				if entry.next.find(segments[1:], result) {
					result.rawWildcards = append(result.rawWildcards, s0[len(entry.prefix):])
					return true
				}

				break
			}
		}
	}

	// There is a catch-all?
	//
	if m.catchAllHandler != nil {
		if (s0 != "") && (s0 != "/") {
			result.Target = m.catchAllHandler
			result.RemainingSegments = segments

			if m.childMiddlewares != nil {
				result.Middlewares = m.catchAllMiddlewaresCache
			}

			return true
		}
	}

	return false
}

func (m *urlResolverPathPart) addPath(segments []string, pathPrefix string, handler any, tag any) {
	m.pathPrefix = pathPrefix

	if len(segments) == 0 {
		m.exactHandler = handler
		m.exactHandlerTag = tag
		m.updateMiddlewaresForMe()
		return
	}

	s0 := segments[0]

	if strings.HasSuffix(s0, "*") {
		// Ends by "/*" then will catch all the urls.
		//
		if (s0 == "*") && (len(segments) == 1) {
			m.catchAllHandler = handler
			m.catchAllHandlerTag = tag
			m.updateMiddlewaresForMe()
			return
		}

		if m.beginByMap == nil {
			m.beginByMap = make(map[string]*urlResolverPathPart)
		}

		m.beginByMapOrdered = nil

		pathPrefix += "/" + s0
		s0 = s0[0 : len(s0)-1]
		current := m.beginByMap[s0]

		if current == nil {
			current = &urlResolverPathPart{parent: m}
			m.beginByMap[s0] = current
		}

		current.addPath(segments[1:], pathPrefix, handler, tag)
		return
	}

	pathPrefix += "/" + s0

	if m.segmentMap == nil {
		m.segmentMap = make(map[string]*urlResolverPathPart)
	} else {
		next := m.segmentMap[s0]

		if next != nil {
			next.addPath(segments[1:], pathPrefix, handler, tag)
			return
		}
	}

	pp := &urlResolverPathPart{parent: m}
	m.segmentMap[s0] = pp
	pp.addPath(segments[1:], pathPrefix, handler, tag)
}

func (m *urlResolverPathPart) appendMiddleware(segments []string, pathPrefix string, handler any, tag any, exactMatch bool) {
	m.pathPrefix = pathPrefix

	if len(segments) == 0 {
		m.exactHandlerTag = tag

		if exactMatch {
			m.exactMiddlewares = append(m.exactMiddlewares, handler)
			m.updateMiddlewaresForMe()
		} else {
			m.childMiddlewares = append(m.childMiddlewares, handler)
			m.updateMiddlewaresForChildren()
		}

		return
	}

	s0 := segments[0]

	if strings.HasSuffix(s0, "*") {
		if (s0 == "*") && (len(segments) == 1) {
			// Here the ends /* is removed before calling,
			// so this case must never append.
			//
			return
		}

		if m.beginByMap == nil {
			m.beginByMap = make(map[string]*urlResolverPathPart)
		}

		m.beginByMapOrdered = nil

		pathPrefix += "/" + s0
		s0 = s0[0 : len(s0)-1]
		current := m.beginByMap[s0]

		if current == nil {
			current = &urlResolverPathPart{parent: m}
			m.beginByMap[s0] = current
		}

		current.appendMiddleware(segments[1:], pathPrefix, handler, tag, exactMatch)
		return
	}

	pathPrefix += "/" + s0

	if m.segmentMap == nil {
		m.segmentMap = make(map[string]*urlResolverPathPart)
	} else {
		next := m.segmentMap[s0]

		if next != nil {
			next.appendMiddleware(segments[1:], pathPrefix, handler, tag, exactMatch)
			return
		}
	}

	pp := &urlResolverPathPart{parent: m}
	m.segmentMap[s0] = pp
	pp.appendMiddleware(segments[1:], pathPrefix, handler, tag, exactMatch)
}

func (m *urlResolverPathPart) updateMiddlewaresForMe() {
	p := m.parent
	var parentMiddlewares []any

	for {
		if p == nil {
			break
		}

		if p.childMiddlewares != nil {
			parentMiddlewares = append(p.childMiddlewares, parentMiddlewares...)
		}

		p = p.parent
	}

	if m.exactMiddlewares == nil {
		m.exactMiddlewaresCache = parentMiddlewares
	} else {
		m.exactMiddlewaresCache = append(parentMiddlewares, m.exactMiddlewares...)
	}

	if m.childMiddlewares == nil {
		m.catchAllMiddlewaresCache = parentMiddlewares
	} else {
		m.catchAllMiddlewaresCache = append(parentMiddlewares, m.childMiddlewares...)
	}
}

func (m *urlResolverPathPart) updateMiddlewaresForChildren() {
	m.walk(func(c *urlResolverPathPart) {
		c.updateMiddlewaresForMe()
	})
}
