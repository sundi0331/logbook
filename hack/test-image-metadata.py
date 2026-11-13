"""Regression checks for the registry metadata publication contract."""
from pathlib import Path
import unittest

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
