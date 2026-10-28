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
