#!/usr/bin/env python3

import os
from pathlib import Path
import re
import subprocess
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


if __name__ == "__main__":
    unittest.main()
