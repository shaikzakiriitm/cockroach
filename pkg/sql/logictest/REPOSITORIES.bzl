# DO NOT EDIT THIS FILE MANUALLY! Use `release update-releases-file`.
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

CONFIG_LINUX_AMD64 = "linux-amd64"
CONFIG_LINUX_ARM64 = "linux-arm64"
CONFIG_DARWIN_AMD64 = "darwin-10.9-amd64"
CONFIG_DARWIN_ARM64 = "darwin-11.0-arm64"

_CONFIGS = [
    ("24.3.6", [
        (CONFIG_DARWIN_AMD64, "775982f9796d9af6d09fd209d8e78b90c18bf820b1a4e7ae9420e2c606774f8b"),
        (CONFIG_DARWIN_ARM64, "1942a15b5ed4fc7bba4edf3d5a47372da4b7d0b82aeb8ddd1a726da5cd2bec8e"),
        (CONFIG_LINUX_AMD64, "02e7f6b93e3a75c2b710aa337728899715cb4a341b70fff7116f149174422ce8"),
        (CONFIG_LINUX_ARM64, "7a801a16bb9c9e718425f661d7ecec27ff65874bec50f996909de57ab474707b"),
    ]),
    ("25.1.0-rc.1", [
        (CONFIG_DARWIN_AMD64, "9669f4710f987f5aa3c9e53bd59260863627d16aa23cb70f1eb2896513472d4f"),
        (CONFIG_DARWIN_ARM64, "39510f7c25b0f0caeb2ecd917eb12fbb65c0ba3a76f63a00189d8d2c9c562e1a"),
        (CONFIG_LINUX_AMD64, "e4b20c7ea368eaea1257e20e7004302320d2a401cddff079eaea3a5d9c9cf188"),
        (CONFIG_LINUX_ARM64, "dfc4661b47b4bede9775564a4d56d3a4284abb4984c318d373c3d960cb6f6cf5"),
    ]),
]

def _munge_name(s):
    return s.replace("-", "_").replace(".", "_")

def _repo_name(version, config_name):
    return "cockroach_binary_v{}_{}".format(
        _munge_name(version),
        _munge_name(config_name))

def _file_name(version, config_name):
    return "cockroach-v{}.{}/cockroach".format(
        version, config_name)

def target(config_name):
    targets = []
    for versionAndConfigs in _CONFIGS:
        version, _ = versionAndConfigs
        targets.append("@{}//:{}".format(_repo_name(version, config_name),
                                         _file_name(version, config_name)))
    return targets

def cockroach_binaries_for_testing():
    for versionAndConfigs in _CONFIGS:
        version, configs = versionAndConfigs
        for config in configs:
            config_name, shasum = config
            file_name = _file_name(version, config_name)
            http_archive(
                name = _repo_name(version, config_name),
                build_file_content = """exports_files(["{}"])""".format(file_name),
                sha256 = shasum,
                urls = [
                    "https://binaries.cockroachdb.com/{}".format(
                        file_name.removesuffix("/cockroach")) + ".tgz",
                ],
            )
