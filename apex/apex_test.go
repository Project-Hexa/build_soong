// Copyright 2018 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apex

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
)

func testApex(t *testing.T, bp string) *android.TestContext {
	config, buildDir := setup(t)
	defer teardown(buildDir)

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("apex", android.ModuleFactoryAdaptor(ApexBundleFactory))
	ctx.RegisterModuleType("apex_key", android.ModuleFactoryAdaptor(apexKeyFactory))

	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("apex_deps", apexDepsMutator)
		ctx.BottomUp("apex", apexMutator)
	})

	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_library_shared", android.ModuleFactoryAdaptor(cc.LibrarySharedFactory))
	ctx.RegisterModuleType("cc_binary", android.ModuleFactoryAdaptor(cc.BinaryFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc.ObjectFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(cc.LlndkLibraryFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc.ToolchainLibraryFactory))
	ctx.RegisterModuleType("prebuilt_etc", android.ModuleFactoryAdaptor(android.PrebuiltEtcFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", cc.ImageMutator).Parallel()
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("vndk", cc.VndkMutator).Parallel()
		ctx.BottomUp("version", cc.VersionMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
	})

	ctx.Register()

	bp = bp + `
		toolchain_library {
			name: "libcompiler_rt-extras",
			src: "",
			vendor_available: true,
			recovery_available: true,
		}

		toolchain_library {
			name: "libatomic",
			src: "",
			vendor_available: true,
			recovery_available: true,
		}

		toolchain_library {
			name: "libgcc",
			src: "",
			vendor_available: true,
			recovery_available: true,
		}

		toolchain_library {
			name: "libclang_rt.builtins-aarch64-android",
			src: "",
			vendor_available: true,
			recovery_available: true,
		}

		toolchain_library {
			name: "libclang_rt.builtins-arm-android",
			src: "",
			vendor_available: true,
			recovery_available: true,
		}

		cc_object {
			name: "crtbegin_so",
			stl: "none",
			vendor_available: true,
			recovery_available: true,
		}

		cc_object {
			name: "crtend_so",
			stl: "none",
			vendor_available: true,
			recovery_available: true,
		}

		llndk_library {
			name: "libc",
			symbol_file: "",
		}

		llndk_library {
			name: "libm",
			symbol_file: "",
		}

		llndk_library {
			name: "libdl",
			symbol_file: "",
		}
	`

	ctx.MockFileSystem(map[string][]byte{
		"Android.bp":                                []byte(bp),
		"testkey.avbpubkey":                         nil,
		"testkey.pem":                               nil,
		"build/target/product/security":             nil,
		"apex_manifest.json":                        nil,
		"system/sepolicy/apex/myapex-file_contexts": nil,
		"mylib.cpp":                                 nil,
		"myprebuilt":                                nil,
	})
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx
}

func setup(t *testing.T) (config android.Config, buildDir string) {
	buildDir, err := ioutil.TempDir("", "soong_apex_test")
	if err != nil {
		t.Fatal(err)
	}

	config = android.TestArchConfig(buildDir, nil)
	config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("current")
	return
}

func teardown(buildDir string) {
	os.RemoveAll(buildDir)
}

// ensure that 'result' contains 'expected'
func ensureContains(t *testing.T, result string, expected string) {
	if !strings.Contains(result, expected) {
		t.Errorf("%q is not found in %q", expected, result)
	}
}

// ensures that 'result' does not contain 'notExpected'
func ensureNotContains(t *testing.T, result string, notExpected string) {
	if strings.Contains(result, notExpected) {
		t.Errorf("%q is found in %q", notExpected, result)
	}
}

func ensureListContains(t *testing.T, result []string, expected string) {
	if !android.InList(expected, result) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}

func ensureListNotContains(t *testing.T, result []string, notExpected string) {
	if android.InList(notExpected, result) {
		t.Errorf("%q is found in %v", notExpected, result)
	}
}

// Minimal test
func TestBasicApex(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that apex variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib2.so")
}

func TestBasicZipApex(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			payload_type: "zip",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	zipApexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("zipApexRule")
	copyCmds := zipApexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, zipApexRule.Output.String(), "myapex.zipapex.unsigned")

	// Ensure that APEX variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that APEX variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_core_shared_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.zipapex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.zipapex/lib64/mylib2.so")
}

func TestApexWithStubs(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib3"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2", "mylib3"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			cflags: ["-include mylib.h"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
		}

		cc_library {
			name: "mylib3",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib4"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "11", "12"],
			},
		}

		cc_library {
			name: "mylib4",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")

	// Ensure that direct stubs dep is included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib3.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_shared_myapex").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with the latest version of stubs for mylib2
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_core_shared_3_myapex/mylib2.so")
	// ... and not linking to the non-stub (impl) variant of mylib2
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_core_shared_myapex/mylib2.so")

	// Ensure that mylib is linking with the non-stub (impl) of mylib3 (because mylib3 is in the same apex)
	ensureContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_core_shared_myapex/mylib3.so")
	// .. and not linking to the stubs variant of mylib3
	ensureNotContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_core_shared_12_myapex/mylib3.so")

	// Ensure that stubs libs are built without -include flags
	mylib2Cflags := ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_core_static_myapex").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylib2Cflags, "-include ")

	// Ensure that genstub is invoked with --apex
	ensureContains(t, "--apex", ctx.ModuleForTests("mylib2", "android_arm64_armv8-a_core_static_3_myapex").Rule("genStubSrc").Args["flags"])
}

func TestApexWithExplicitStubsDependency(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo#10"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "libfoo",
			srcs: ["mylib.cpp"],
			shared_libs: ["libbar"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "20", "30"],
			},
		}

		cc_library {
			name: "libbar",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}

	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.so")

	// Ensure that dependency of stubs is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libbar.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_shared_myapex").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with version 10 of libfoo
	ensureContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_core_shared_10_myapex/libfoo.so")
	// ... and not linking to the non-stub (impl) variant of libfoo
	ensureNotContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_core_shared_myapex/libfoo.so")

	libFooStubsLdFlags := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_core_shared_10_myapex").Rule("ld").Args["libFlags"]

	// Ensure that libfoo stubs is not linking to libbar (since it is a stubs)
	ensureNotContains(t, libFooStubsLdFlags, "libbar.so")
}

func TestApexWithSystemLibsStubs(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib_shared", "libdl", "libm"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
		}

		cc_library_shared {
			name: "mylib_shared",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
		}

		cc_library {
			name: "libc",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
		}

		cc_library {
			name: "libm",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
		}

		cc_library {
			name: "libdl",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that mylib, libm, libdl are included.
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libm.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libdl.so")

	// Ensure that libc is not included (since it has stubs and not listed in native_shared_libs)
	ensureNotContains(t, copyCmds, "image.apex/lib64/libc.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_shared_myapex").Rule("ld").Args["libFlags"]
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_core_static_myapex").Rule("cc").Args["cFlags"]
	mylibSharedCFlags := ctx.ModuleForTests("mylib_shared", "android_arm64_armv8-a_core_shared_myapex").Rule("cc").Args["cFlags"]

	// For dependency to libc
	// Ensure that mylib is linking with the latest version of stubs
	ensureContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_core_shared_29_myapex/libc.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_core_shared_myapex/libc.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBC_API__=29")
	ensureContains(t, mylibSharedCFlags, "__LIBC_API__=29")

	// For dependency to libm
	// Ensure that mylib is linking with the non-stub (impl) variant
	ensureContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_core_shared_myapex/libm.so")
	// ... and not linking to the stub variant
	ensureNotContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_core_shared_29_myapex/libm.so")
	// ... and is not compiling with the stub
	ensureNotContains(t, mylibCFlags, "__LIBM_API__=29")
	ensureNotContains(t, mylibSharedCFlags, "__LIBM_API__=29")

	// For dependency to libdl
	// Ensure that mylib is linking with the specified version of stubs
	ensureContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_27_myapex/libdl.so")
	// ... and not linking to the other versions of stubs
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_28_myapex/libdl.so")
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_29_myapex/libdl.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_core_shared_myapex/libdl.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBDL_API__=27")
	ensureContains(t, mylibSharedCFlags, "__LIBDL_API__=27")
}

func TestFilesInSubDir(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			prebuilts: ["myetc"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		prebuilt_etc {
			name: "myetc",
			src: "myprebuilt",
			sub_dir: "foo/bar",
		}
	`)

	generateFsRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("generateFsConfig")
	dirs := strings.Split(generateFsRule.Args["exec_paths"], " ")

	// Ensure that etc, etc/foo, and etc/foo/bar are all listed
	ensureListContains(t, dirs, "etc")
	ensureListContains(t, dirs, "etc/foo")
	ensureListContains(t, dirs, "etc/foo/bar")
}

func TestUseVendor(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			use_vendor: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			vendor_available: true,
			stl: "none",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			vendor_available: true,
			stl: "none",
		}
	`)

	inputsList := []string{}
	for _, i := range ctx.ModuleForTests("myapex", "android_common_myapex").Module().BuildParamsForTests() {
		for _, implicit := range i.Implicits {
			inputsList = append(inputsList, implicit.String())
		}
	}
	inputsString := strings.Join(inputsList, " ")

	// ensure that the apex includes vendor variants of the direct and indirect deps
	ensureContains(t, inputsString, "android_arm64_armv8-a_vendor_shared_myapex/mylib.so")
	ensureContains(t, inputsString, "android_arm64_armv8-a_vendor_shared_myapex/mylib2.so")

	// ensure that the apex does not include core variants
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_core_shared_myapex/mylib.so")
	ensureNotContains(t, inputsString, "android_arm64_armv8-a_core_shared_myapex/mylib2.so")
}

func TestStaticLinking(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
		}

		cc_binary {
			name: "not_in_apex",
			srcs: ["mylib.cpp"],
			static_libs: ["mylib"],
			static_executable: true,
			system_shared_libs: [],
			stl: "none",
		}

		cc_object {
			name: "crtbegin_static",
			stl: "none",
		}

		cc_object {
			name: "crtend_android",
			stl: "none",
		}

	`)

	ldFlags := ctx.ModuleForTests("not_in_apex", "android_arm64_armv8-a_core").Rule("ld").Args["libFlags"]

	// Ensure that not_in_apex is linking with the static variant of mylib
	ensureContains(t, ldFlags, "mylib/android_arm64_armv8-a_core_static_platform/mylib.a")
}
