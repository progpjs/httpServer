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
	"strings"
	"testing"
	"time"
)

//region Tests requirements

var gUrlResolver *UrlResolver
var gTest *testing.T

func addPath(path string) {
	gUrlResolver.Add(path, path, "tag:"+path)
}

func addMiddleware(path string) {
	gUrlResolver.AppendMiddleware(path, path, "tag:"+path)
}

func expectFound(testing string, samplePath string) *UrlResolverResult {
	res := gUrlResolver.Find(samplePath)

	if res.Target == nil {
		gTest.Error("Not found [", samplePath, "]. Was testing rule [", testing, "]")
		return nil
	}

	execRes := res.Target.(string)

	if testing != execRes {
		gTest.Error("Match the bad rule.\n- Matched [", execRes, "]\n- Expected [", testing, "]")
		return nil
	}

	return &res
}

func expectNotFound(testing string, samplePath string) {
	res := gUrlResolver.Find(samplePath)

	if res.Target != nil {
		gTest.Error("Should NOT found: ", samplePath, "]\n- Was testing [", testing, "]\n- Returned rule is [", res, "]")
		return
	}
}

func expectWildcards(testing string, samplePath string, expectWildcards string, expectedRemaining string) *UrlResolverResult {
	res := expectFound(testing, samplePath)
	if res == nil {
		return nil
	}

	foundWildcards := ""
	if res.rawWildcards != nil {
		foundWildcards = strings.Join(res.GetWildcards(), "/")
	}

	if expectWildcards != foundWildcards {
		gTest.Error("Invalid wildcard for url [", samplePath, "]",
			"\n- Was testing rule [", testing, "]",
			"\n- Found wildcards [", foundWildcards, "]",
			"\n- Expected wildcards [", expectWildcards, "]")

		return nil
	}

	foundRemaining := ""

	if foundRemaining != expectedRemaining {
		gTest.Error("Invalid remaining segments for url [", samplePath, "]",
			"\n- Was testing rule [", testing, "]",
			"\n- Found segments [", foundRemaining, "]",
			"\n- Expected segments [", expectedRemaining, "]")

		return nil
	}

	return res
}

func expectMiddleware(samplePath string, middlewares []string) {
	res := gUrlResolver.Find(samplePath)

	if len(res.Middlewares) != len(middlewares) {
		gTest.Error("Invalid middleware counter for url [", samplePath, "]",
			"\n- Found exactMiddlewares counter [", len(res.Middlewares), "]",
			"\n- Expected exactMiddlewares counter [", len(middlewares), "]")

		return
	}

	for i := 0; i < len(res.Middlewares); i++ {
		execRes := res.Middlewares[i].(string)

		if execRes != middlewares[i] {
			gTest.Error("Invalid middleware for url [", samplePath, "] at index", i,
				"\n- Found exactMiddlewares [", execRes, "]",
				"\n- Expected exactMiddlewares [", middlewares[i], "]")

			return
		}
	}
}

func printDumpTree() {
	tree := gUrlResolver.DumpTree()

	for _, entry := range tree {
		if entry.Path[0] == '@' {
			println(entry.Path)
		}
	}

	for _, entry := range tree {
		if entry.Path[0] != '@' {
			println(entry.Path)
		}
	}
}

//endregion

func getRuleSet() []string {
	rules := []string{
		"/", "/products", "/products/bedroom",
		"/clients", "/clients/", "/clients/johan",
		"/vip", "/vip/johan",
		"/products/any/*",
		"/products/listing1*", "/products/listing1*/suiteA", "/products/listing1*/suiteB",
		"/products/listing1b*", "/products/listing1b*/suite1B",
		"/products/listing2*", "/products/listing2", "/products/listing2/",
		"/wildcards/w1*/suite/w2*", "/wildcards/w1*/suite/w2*/*",
	}

	sort.Slice(rules, func(i, j int) bool {
		return rules[i] < rules[j]
	})

	return rules
}

func buildPathSet() {
	urlResolver := NewUrlResolver()
	gUrlResolver = urlResolver

	rules := getRuleSet()

	for _, rule := range rules {
		addPath(rule)
	}
}

func addMiddlewares() {
	// >>> Exact middleware

	addMiddleware("/clients")
	addMiddleware("/clients/")
	addMiddleware("/clients/johan")
	addMiddleware("/products/listing1*/suiteA")
	addMiddleware("/wildcards/w1*/suite/w2*")
	addMiddleware("/wildcards/w1*/suite/w2*/*")
	addMiddleware("/wildcards/*")

	// >>> Middleware from

	// Doesn't include any and any/
	addMiddleware("/products/any/*")
}

func doAssertions() {
	// >>> Test expected cases

	expectFound("/", "/")
	expectFound("/products", "/products")
	expectFound("/products/bedroom", "/products/bedroom")

	expectFound("/clients", "/clients")
	expectFound("/clients/", "/clients/")
	expectFound("/clients/johan", "/clients/johan")

	expectFound("/vip", "/vip")
	expectFound("/vip/johan", "/vip/johan")

	expectFound("/products/any/*", "/products/any/aa")
	expectFound("/products/any/*", "/products/any/aa/bb")

	expectFound("/products/listing1*", "/products/listing1fff")
	expectFound("/products/listing1*/suiteA", "/products/listing1fff/suiteA")
	expectFound("/products/listing1*/suiteB", "/products/listing1fff/suiteB")

	expectFound("/products/listing1b*", "/products/listing1bfff")
	expectFound("/products/listing1b*/suite1B", "/products/listing1bfff/suite1B")

	expectFound("/products/listing2", "/products/listing2")
	expectFound("/products/listing2/", "/products/listing2/")
	expectFound("/products/listing2*", "/products/listing2fff")

	expectFound("/wildcards/w1*/suite/w2*", "/wildcards/w1WD1/suite/w2WD2")
	expectFound("/wildcards/w1*/suite/w2*/*", "/wildcards/w1WD1/suite/w2WD2/suite")

	// >>> Test unexpected cases

	expectNotFound("/products/listing1*", "/products/listing1")
	expectNotFound("/products/listing2/", "/products/listing2/ko")

	// Must have precedence over "/products/listing1*".
	expectNotFound("/products/listing1b*", "/products/listing1bfff/ko")
	expectNotFound("/products/listing1b*/suite1B", "/products/listing1bfff/suite1B/ko")

	expectNotFound("/clients/", "/clients/ko")

	// When ending by /*, then expect to find something after "/any"
	//
	expectNotFound("/products/any/*", "/products/any")
	expectNotFound("/products/any/*", "/products/any/")

	// >>> Test wildcards

	expectWildcards("/products/listing1*", "/products/listing1MY_WILDCARD", "MY_WILDCARD", "")
	expectWildcards("/products/listing1*/suiteA", "/products/listing1MY_WILDCARD/suiteA", "MY_WILDCARD", "")
	expectWildcards("/products/listing1b*/suite1B", "/products/listing1bMY_WILDCARD/suite1B", "MY_WILDCARD", "")

	expectWildcards("/wildcards/w1*/suite/w2*/*", "/wildcards/w1WD1/suite/w2WD2/suite1/suite2", "WD1/WD2", "suite1/suite2")
}

func doMiddlewaresAssertions() {
	// >>> Exact middlewares

	expectMiddleware("/clients", []string{"/clients"})
	expectMiddleware("/clients/", []string{"/clients/"})
	expectMiddleware("/clients/johan", []string{"/clients/johan"})
	expectMiddleware("/products/listing1*/suiteA", []string{"/products/listing1*/suiteA"})
	expectMiddleware("/wildcards/w1*/suite/w2*", []string{"/wildcards/*", "/wildcards/w1*/suite/w2*"})

	expectMiddleware("/clients/unknown", []string{})

	// >>> Middleware from

	expectMiddleware("/products", []string{})
	expectMiddleware("/products/", []string{})
	expectMiddleware("/products/any", []string{})
	expectMiddleware("/products/any/", []string{})

	expectMiddleware("/products/any/p1", []string{"/products/any/*"})
	expectMiddleware("/products/any/p1/p2", []string{"/products/any/*"})
}

func TestResolving(test *testing.T) {
	gTest = test
	buildPathSet()
	addMiddlewares()
	doAssertions()
}

func TestMiddlewares(test *testing.T) {
	gTest = test
	buildPathSet()
	addMiddlewares()

	doMiddlewaresAssertions()
}

func TestBenchmark(test *testing.T) {
	// Score before refactoring:
	//		5000000  tests executed in  1109 ms.
	// Score after refactoring:
	//		5000000  tests executed in  969 ms.

	repeatCount := 5000000
	toTest := "/wildcards/w1*/suite/w2*/suite/aaa"
	// toTest = "/"

	gTest = test
	buildPathSet()
	addMiddlewares()

	timeBefore := time.Now()

	for i := 0; i < repeatCount; i++ {
		gUrlResolver.Find(toTest)
	}

	timeDiff := time.Now().Sub(timeBefore)

	totalMs := int(timeDiff.Milliseconds())
	countPerSec := (repeatCount * 1000) / totalMs
	countPerMs := repeatCount / totalMs

	println(repeatCount, " tests executed in ", totalMs, "ms. ")
	println("=", countPerSec, "/sec")
	println("=", countPerMs, "/ms")
}

func TestTreeDumping(test *testing.T) {
	gTest = test

	// >>> Build the resolver and export it

	buildPathSet()

	tree := gUrlResolver.DumpTree()

	// >>> Check the result's tags

	for _, entry := range tree {
		if (entry.Path[0] != '@') && (entry.Tag != "tag:"+entry.Path) {
			test.Error("Tree dumping mismatch for tag", "\n|- Expected tag:", "tag:"+entry.Path, "\n|- Found tag:", entry.Tag)
			return
		}
	}

	// >>> Check the result's paths

	var resultPaths []string

	for _, entry := range tree {
		if entry.Path[0] != '@' {
			resultPaths = append(resultPaths, entry.Path)
		}
	}

	sort.Slice(resultPaths, func(i, j int) bool {
		return resultPaths[i] < resultPaths[j]
	})

	rules := getRuleSet()

	if len(rules) != len(resultPaths) {
		test.Error("Tree dumping size don't match.\n|- Result size is", len(resultPaths), "\n|- Expected size is", len(rules))
		return
	}

	for i := 0; i < len(rules); i++ {
		a := rules[i]
		b := resultPaths[i]

		if a != b {
			test.Error("Tree dumping mismatch at index", i)
			return
		}
	}

	// >>> Create a resolver from the result and test it

	urlResolver := NewUrlResolver()
	gUrlResolver = urlResolver

	for _, entry := range tree {
		urlResolver.Add(entry.Path, entry.Handler, "tag:"+entry.Path)
	}

	doAssertions()

	// >>> Test again with middlewares

	addMiddlewares()
	tree = urlResolver.DumpTree()

	//printDumpTree()

	urlResolver = NewUrlResolver()
	gUrlResolver = urlResolver

	for _, entry := range tree {
		if entry.Path[0] == '@' {
			urlResolver.AppendMiddleware(entry.Path[1:], entry.Handler, "tag:"+entry.Path[1:])
		} else {
			urlResolver.Add(entry.Path, entry.Handler, "tag:"+entry.Path)
		}
	}

	doAssertions()
	doMiddlewaresAssertions()
}

//endregion
