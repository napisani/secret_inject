#!/usr/bin/env python3

import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest


class MigrationWizardTest(unittest.TestCase):
    def test_local_readiness_does_not_require_release_certification(self):
        wizard = Path(__file__).with_name("migrate-private-credentials.sh").read_text()
        stages = list(re.finditer(r"^stage ", wizard, re.MULTILINE))
        readiness = wizard[stages[0].start():stages[1].start()]
        with tempfile.TemporaryDirectory(prefix="secret-inject-readiness-") as temporary:
            root = Path(temporary)
            trace = root / "go-arguments"
            go = root / "go"
            go.write_text(
                "#!/bin/sh\n"
                'printf "%s\\n" "$@" > "$READINESS_ARGS"\n'
                'case " $* " in *releasegate*) exit 1 ;; esac\n'
                'test "$GOTOOLCHAIN" = go1.25.0\n'
            )
            go.chmod(0o700)
            script = (
                "set -eu\n"
                "stage() { :; }; say() { :; }; warn() { :; }; confirm() { return 0; }\n"
                + readiness
            )
            result = subprocess.run(
                ["/bin/bash", "-c", script],
                env={
                    **os.environ,
                    "PATH": str(root),
                    "PROJECT": str(root),
                    "DOC": str(root / "conformance.md"),
                    "READINESS_ARGS": str(trace),
                },
                capture_output=True,
                text=True,
                timeout=10,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(trace.read_text().splitlines(), ["test", "-count=1", "./..."])

    def test_managed_configs_select_age_on_darwin_and_systemd_on_linux(self):
        wizard = Path(__file__).with_name("migrate-private-credentials.sh").read_text()
        selection_start = wizard.index('case "$(uname -s)" in')
        selection_end = wizard.index("\nesac", selection_start) + len("\nesac")
        start = wizard.index("stage 'Choose the personal or work configuration'")
        end = wizard.index("stage 'Preserve the existing configuration privately'", start)
        repo = Path(__file__).resolve().parents[3]
        for system, backend in (("Darwin", "darwin-age"), ("Linux", "linux-systemd-credentials")):
            for scope in ("personal", "work"):
                with self.subTest(system=system, scope=scope):
                    with tempfile.TemporaryDirectory(prefix="secret-inject-managed-config-") as temporary:
                        root = Path(temporary)
                        state = root / "state" / "secret_inject" / "migration"
                        script = (
                            "set -eu\n"
                            "stage() { :; }; say() { :; }; warn() { :; }; pause() { :; }; write_env() { :; }\n"
                            'uname() { printf "%s\\n" "$FIXTURE_SYSTEM"; }\n'
                            'ask() { printf -v "$1" "%s" "$FIXTURE_SCOPE"; }\n'
                            + wizard[selection_start:selection_end] + "\n"
                            + wizard[start:end]
                            + '\nprintf "%s" "$BACKEND" > "$SELECTED_BACKEND"\n'
                        )
                        result = subprocess.run(
                            ["/bin/bash", "-c", script],
                            env={
                                **os.environ,
                                "FIXTURE_SYSTEM": system,
                                "FIXTURE_SCOPE": scope,
                                "SELECTED_BACKEND": str(root / "backend"),
                                "STATE_DIR": str(state),
                                "ENV_FILE": str(state / "choices.env"),
                                "PYTHON": sys.executable,
                                "REPO": str(repo),
                            },
                            capture_output=True, text=True, timeout=10,
                        )
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertEqual((root / "backend").read_text(), backend)
                        staged = list(state.glob("run.*/native-config.json"))
                        self.assertEqual(len(staged), 1)
                        config = json.loads(staged[0].read_text())
                        self.assertIn(backend, config["storage"]["allowed_backends"])
                        self.assertEqual(config["storage"]["allowed_backends"],
                                         ["darwin-age", "linux-systemd-credentials"])

    def test_age_setup_warns_before_preparation(self):
        wizard = Path(__file__).with_name("migrate-private-credentials.sh").read_text()
        start = wizard.index("stage 'Explicitly prepare the selected protected store'")
        end = wizard.index("stage 'Enroll or keep the provider-typed Doppler record'", start)
        with tempfile.TemporaryDirectory(prefix="secret-inject-age-warning-") as temporary:
            root = Path(temporary)
            trace = root / "trace"
            cli = root / "cli"
            cli.write_text('#!/bin/sh\nprintf "prepare\\n" >> "$TRACE"\n')
            cli.chmod(0o700)
            script = (
                "set -eu\n"
                "stage() { :; }; say() { :; }; pause() { :; }\n"
                'warn() { printf "%s\\n" "$*" >> "$TRACE"; }\n'
                'confirm() { printf "confirm\\n" >> "$TRACE"; }\n'
                + wizard[start:end]
            )
            result = subprocess.run(
                ["/bin/bash", "-c", script],
                env={**os.environ, "BACKEND": "darwin-age", "SI": str(cli),
                     "CFG": str(root / "config.json"), "TRACE": str(trace)},
                capture_output=True, text=True, timeout=10,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            events = trace.read_text().splitlines()
            self.assertIn("identity is readable by your user", events[0])
            self.assertIn("weaker key custody than Keychain", events[1])
            self.assertEqual(events[2:], ["confirm", "prepare"])

    def test_installed_binary_prepares_access_before_cache_only_check(self):
        wizard = Path(__file__).with_name("migrate-private-credentials.sh").read_text()
        start = wizard.index("stage 'Verify the activated config, binary, and fresh-shell startup'")
        end = wizard.index("stage 'Optional legacy-cache cleanup", start)
        with tempfile.TemporaryDirectory(prefix="secret-inject-installed-access-") as temporary:
            root = Path(temporary)
            run = root / "run"
            run.mkdir()
            trace = root / "trace"
            ready = root / "ready"
            config = root / "config.json"
            config.write_text(json.dumps({"storage": {"type": "keyring"}}))
            installed = root / "installed"
            installed.write_text(
                "#!/bin/bash\n"
                'if [[ "$1" == --help ]]; then echo "credential set/list/remove"; exit; fi\n'
                '[[ "$1 $2" == "store prepare" ]] || exit 2\n'
                'printf "prepare\\n" >> "$TRACE"\n'
                ': > "$READY"\n'
            )
            installed.chmod(0o700)
            script = (
                "set -eu\n"
                "stage() { :; }; say() { :; }; warn() { :; }; step() { :; }\n"
                "confirm() { return 0; }; write_env() { :; }\n"
                'ask() { printf -v "$1" "%s" "$FAKE_INSTALLED"; }\n'
                'verify_cached_export() { printf "cache-check\\n" >> "$TRACE"; '
                '[[ -f "$READY" ]] || { echo "protected store requires explicit preparation" >&2; return 1; }; }\n'
                + wizard[start:end]
            )
            result = subprocess.run(
                ["/bin/bash", "-c", script],
                env={
                    **os.environ,
                    "BACKEND": "linux-systemd-credentials",
                    "RUN_DIR": str(run),
                    "CFG": str(config),
                    "ACTIVE_CONFIG": str(config),
                    "PYTHON": sys.executable,
                    "FAKE_INSTALLED": str(installed),
                    "TRACE": str(trace),
                    "READY": str(ready),
                },
                capture_output=True,
                text=True,
                timeout=10,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(trace.read_text().splitlines(), ["prepare", "cache-check"])


if __name__ == "__main__":
    unittest.main()
