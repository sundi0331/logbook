"""Regression checks for the registry metadata publication contract."""
from pathlib import Path
import unittest
import os
import subprocess
import tempfile
import textwrap

ROOT = Path(__file__).resolve().parents[1]


class ImageMetadataTests(unittest.TestCase):
    def test_git_context_tracks_selected_source(self):
        container = (ROOT / ".github/workflows/container-image.yml").read_text()
        release = (ROOT / ".github/workflows/release.yml").read_text()
        self.assertNotIn("context:", container)
        self.assertNotIn("context: .", release)
        build = release.split("id: build-image\n")[1].split("\n      - name:")[0]
        self.assertIn(
            "context: https://github.com/${{ github.repository }}.git#${{ steps.metadata.outputs.commit }}",
            build,
        )
        self.assertLess(release.index('git checkout --detach "refs/tags/${VERSION}"'),
                        release.index('COMMIT="$(git rev-parse HEAD)"'))

    def test_signed_provenance_is_published_for_image_digest(self):
        for filename, build in [("container-image.yml", "build"), ("release.yml", "build-image")]:
            with self.subTest(workflow=filename):
                workflow = (ROOT / ".github/workflows" / filename).read_text()
                self.assertIn("attestations: write", workflow)
                self.assertIn("artifact-metadata: write", workflow)
                step = workflow.split("- name: Attest signed image provenance\n")[1].split("\n      - name:")[0]
                self.assertIn("uses: actions/attest@v4", step)
                self.assertIn("subject-name: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}", step)
                self.assertIn("subject-digest: ${{ steps." + build + ".outputs.digest }}", step)
                self.assertIn("push-to-registry: true", step)
                self.assertNotIn("predicate-type:", step)

    def test_image_signed_once_and_verified_after_attestations(self):
        for filename, build, sign_name in [
            ("container-image.yml", "build", "Sign image"),
            ("release.yml", "build-image", "Sign release image"),
        ]:
            with self.subTest(workflow=filename):
                workflow = (ROOT / ".github/workflows" / filename).read_text()
                sign = workflow.split("- name: " + sign_name + "\n")[1].split("\n      - name:")[0]
                verify = workflow.split("- name: Verify image signature\n")[1].split("\n      - name:")[0]
                self.assertNotIn("TAGS:", sign)
                for step in (sign, verify):
                    self.assertIn("steps." + build + ".outputs.digest", step)
                self.assertLess(workflow.index("--predicate grype.json"), workflow.index("- name: Verify image signature"))
                self.assertLess(workflow.index("uses: actions/attest@v4"), workflow.index("- name: Verify image signature"))
                self.assertLess(workflow.index("- name: " + sign_name), workflow.index("- name: Verify image signature"))
                # Run the actual shell blocks with a stub, checking exact arguments
                # for both tag-triggered and manually dispatched releases.
                with tempfile.TemporaryDirectory() as directory:
                    stub = Path(directory) / "cosign"
                    stub.write_text('#!/bin/bash\nprintf "%s\\n" "$@" >> "$CALL_LOG"\nexit "${VERIFY_EXIT:-0}"\n')
                    stub.chmod(0o755)
                    log = Path(directory) / "calls"
                    for ref in ("refs/heads/main", "refs/tags/v1.2.3"):
                        env = dict(os.environ, PATH=directory + ":" + os.environ["PATH"],
                                   CALL_LOG=str(log), REGISTRY="ghcr.io", IMAGE_NAME="example/project",
                                   DIGEST="sha256:" + "a" * 64, GITHUB_SERVER_URL="https://github.com",
                                   GITHUB_WORKFLOW_REF="example/project/.github/workflows/" + filename + "@" + ref)
                        image = env["REGISTRY"] + "/" + env["IMAGE_NAME"] + "@" + env["DIGEST"]
                        log.write_text("")
                        subprocess.run(["bash", "-c", textwrap.dedent(sign.split("run: |\n")[1])], env=env, check=True)
                        self.assertEqual(log.read_text().splitlines(), ["sign", "--yes", image])
                        log.write_text("")
                        script = textwrap.dedent(verify.split("run: |\n")[1])
                        subprocess.run(["bash", "-c", script], env=env, check=True)
                        self.assertEqual(log.read_text().splitlines(), [
                            "verify", "--certificate-identity", "https://github.com/" + env["GITHUB_WORKFLOW_REF"],
                            "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", image,
                        ])
                        env["VERIFY_EXIT"] = "1"
                        self.assertNotEqual(subprocess.run(["bash", "-c", script], env=env).returncode, 0)

    def test_published_images_have_all_metadata(self):
        for filename, build in [("container-image.yml", "build"), ("release.yml", "build-image")]:
            with self.subTest(workflow=filename):
                workflow = (ROOT / ".github/workflows" / filename).read_text()
                self.assertIn("provenance: mode=max,version=v1\n", workflow)
                self.assertIn("-o spdx-json=sbom.spdx.json", workflow)
                self.assertIn("sbom: sbom.spdx.json", workflow)
                self.assertIn("output-format: json", workflow)
                self.assertIn("output-file: grype.json", workflow)
                self.assertIn("fail-build: false", workflow)
                step = workflow.split("- name: Attest image vulnerabilities\n")[1].split("\n      - name:")[0]
                self.assertIn("steps." + build + ".outputs.digest", step)
                self.assertIn('--predicate grype.json --type https://anchore.com/grype/scan/v1', step)
                self.assertIn('"${REGISTRY}/${IMAGE_NAME}@${DIGEST}"', step)
                self.assertLess(workflow.index("output-file: grype.json"), workflow.index("--predicate grype.json"))


if __name__ == "__main__":
    unittest.main()
