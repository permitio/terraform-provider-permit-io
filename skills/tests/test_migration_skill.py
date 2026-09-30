"""Tests for the terraform-provider-permit-io-1-migration skill.

Run from anywhere with the standard library only:

    python3 -m unittest discover -s skills/tests -v

The fixture configurations are stored as *.fixture so that Terraform tooling and
the provider's example tests don't read them; each test copies them into a
temporary directory without that suffix. In a fixture, a comment
"# expect: ID SAFETY" names a finding the scanner must report on that line, and
"# expect-next: ID SAFETY" one on the next line. Every other finding fails.
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

TESTS = Path(__file__).resolve().parent
REPO = TESTS.parents[1]
SKILL = REPO / "skills" / "terraform-provider-permit-io-1-migration"
SCANNER = SKILL / "scripts" / "scan.py"
CHANGES = SKILL / "references" / "changes.md"
SKILL_MD = SKILL / "SKILL.md"
GUIDE = REPO / "docs" / "guides" / "version-1-upgrade.md"
FIXTURES = TESTS / "fixtures"
GUIDE_URL = (
    "https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/"
    "version-1-upgrade"
)
SAFETIES = ("SAFE", "NEEDS-REVIEW", "INFO")
MARKER_RE = re.compile(r"#\s*expect(-next)?:(.*)$")
PAIR_RE = re.compile(r"\b([A-Z]+\d*)\s+(SAFE|NEEDS-REVIEW|INFO)\b")
ENTRY_HEADING_RE = re.compile(r"^### ([A-Z]+\d*)\. (.+)$")
BUCKET_HEADING_RE = re.compile(r"^## ([A-Z]+): (.+)$")
NOT_NONE_WORDS = ("replacement", "create", "update", "error")


def load_scanner():
    spec = importlib.util.spec_from_file_location("migration_scan", SCANNER)
    if spec is None or spec.loader is None:
        raise ImportError(f"cannot load the scanner from {SCANNER}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


scan = load_scanner()


def materialize(name: str, destination: Path) -> Path:
    """Copies a fixture tree into destination, dropping the .fixture suffixes."""
    target = destination / name
    for source in sorted((FIXTURES / name).rglob("*.fixture")):
        relative = source.relative_to(FIXTURES / name)
        path = target / relative.parent / relative.name[: -len(".fixture")]
        path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, path)
    if name == "v0":
        vendored = target / ".terraform" / "modules" / "vendored" / "main.tf"
        vendored.parent.mkdir(parents=True)
        shutil.copyfile(FIXTURES / "vendored.tf.fixture", vendored)
    return target


def expected_findings(root: Path) -> set[tuple[str, int, str, str]]:
    expected = set()
    for path in sorted(p for p in root.rglob("*") if p.is_file()):
        relative = path.relative_to(root).as_posix()
        if relative.startswith(".terraform/"):
            continue
        for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
            marker = MARKER_RE.search(line)
            if marker:
                target = number + 1 if marker.group(1) else number
                for entry, safety in PAIR_RE.findall(marker.group(2)):
                    expected.add((relative, target, entry, safety))
    return expected


def run_scanner(*arguments: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, str(SCANNER), *arguments],
        capture_output=True,
        text=True,
        check=False,
        timeout=60,
    )


def scan_json(root: Path) -> tuple[int, dict]:
    result = run_scanner(str(root), "--json")
    return result.returncode, json.loads(result.stdout)


def found(document: dict) -> set[tuple[str, int, str, str]]:
    return {(f["path"], f["line"], f["id"], f["safety"]) for f in document["findings"]}


def slug(heading: str) -> str:
    """The anchor that the registry and GitHub give a Markdown heading."""
    text = heading.strip().lower()
    text = re.sub(r"[^\w\- ]", "", text)
    return text.replace(" ", "-")


def guide_entries() -> dict[str, str]:
    """Maps each guide entry ID to its heading anchor.

    An entry is a "### ID. Title" heading, or a "## ID: Title" bucket heading
    with no entries of its own (D, the deprecations).
    """
    entries = {}
    bucket = None
    for line in GUIDE.read_text(encoding="utf-8").splitlines():
        entry = ENTRY_HEADING_RE.match(line)
        heading = BUCKET_HEADING_RE.match(line)
        if entry:
            entries[entry.group(1)] = slug(line[4:])
            bucket = None
        elif heading or line.startswith("## "):
            if bucket:
                entries[bucket[0]] = bucket[1]
            bucket = (heading.group(1), slug(line[3:])) if heading else None
    if bucket:
        entries[bucket[0]] = bucket[1]
    return entries


def guide_plan_impacts() -> dict[str, str]:
    """Maps each ID in the guide's summary table to its plan impact."""
    impacts = {}
    for line in GUIDE.read_text(encoding="utf-8").splitlines():
        if not line.startswith("| ["):
            continue
        cells = [cell.strip() for cell in line.strip().strip("|").split("|")]
        for entry in re.findall(r"\[([A-Z]+\d*)\]\(#", cells[0]):
            impacts[entry] = cells[-1]
    return impacts


def is_plain_none(impact: str | None) -> bool:
    lowered = (impact or "").lower()
    return lowered.startswith("none") and not any(word in lowered for word in NOT_NONE_WORDS)


def changes_sections() -> dict[str, str]:
    """Maps each "### ID. Title" section of references/changes.md to its text."""
    sections = {}
    current = None
    for line in CHANGES.read_text(encoding="utf-8").splitlines():
        heading = ENTRY_HEADING_RE.match(line)
        if heading:
            current = heading.group(1)
            sections[current] = ""
        elif line.startswith("## ") or line.startswith("# "):
            current = None
        elif current:
            sections[current] += line + "\n"
    return sections


def field_line(section: str, name: str) -> str | None:
    match = re.search(rf"^- {name}:(.*(?:\n  .*)*)", section, re.M)
    return match.group(1) if match else None


class FixtureScanTest(unittest.TestCase):
    """The scanner reports exactly the marked findings, and nothing unsafe as SAFE."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.temp = tempfile.TemporaryDirectory()
        cls.results = {}
        for name in ("v0", "v1", "broken"):
            root = materialize(name, Path(cls.temp.name))
            code, document = scan_json(root)
            cls.results[name] = (root, code, document)

    @classmethod
    def tearDownClass(cls) -> None:
        cls.temp.cleanup()

    def assert_markers(self, name: str) -> None:
        root, _, document = self.results[name]
        expected = expected_findings(root)
        actual = found(document)
        self.assertTrue(expected, f"the {name} fixture marks no finding")
        self.assertEqual(
            sorted(expected - actual), [], f"{name}: marked findings the scanner missed"
        )
        self.assertEqual(sorted(actual - expected), [], f"{name}: findings that no marker expects")

    def test_v0_reports_every_marked_finding_at_its_line(self) -> None:
        _, code, document = self.results["v0"]
        self.assertEqual(code, 0, document["warnings"])
        self.assertTrue(document["complete"])
        self.assert_markers("v0")

    def test_v1_reports_only_informational_items(self) -> None:
        _, code, document = self.results["v1"]
        self.assertEqual(code, 0, document["warnings"])
        self.assertEqual(document["warnings"], [])
        not_info = [f for f in document["findings"] if f["safety"] != "INFO"]
        self.assertEqual(not_info, [], "a 1.0 configuration got SAFE or NEEDS-REVIEW findings")
        self.assert_markers("v1")

    def test_every_detectable_guide_entry_is_detected(self) -> None:
        reported = found(self.results["v0"][2]) | found(self.results["v1"][2])
        pairs = {(entry, safety) for _, _, entry, safety in reported}
        declared = {(entry, safety) for entry, values in scan.RULES.items() for safety in values}
        self.assertEqual(sorted(declared - pairs), [], "RULES entries no fixture exercises")
        self.assertEqual(sorted(pairs - declared), [], "findings that RULES doesn't declare")
        self.assertTrue(set(scan.RULES) <= {entry for entry, _ in pairs})

    def test_no_change_that_can_replace_or_fail_is_safe(self) -> None:
        impacts = guide_plan_impacts()
        self.assertTrue(
            any("Replacement" in impact for impact in impacts.values()),
            "no replacement entries read from the guide's summary table",
        )
        for entry, safeties in scan.RULES.items():
            if "SAFE" in safeties:
                self.assertTrue(
                    is_plain_none(impacts.get(entry)),
                    f"{entry} can be SAFE, but its plan impact is {impacts.get(entry)!r}",
                )
        for name in ("v0", "v1"):
            for finding in self.results[name][2]["findings"]:
                if finding["safety"] == "SAFE":
                    self.assertTrue(
                        is_plain_none(impacts.get(finding["id"])),
                        f"{name}: SAFE finding for {finding['id']}, whose plan impact is "
                        f"{impacts.get(finding['id'])!r}: {finding}",
                    )

    def test_summary_reads_the_lock_file_backend_and_required_version(self) -> None:
        summary = self.results["v0"][2]["summary"]
        self.assertEqual(
            summary["locked_versions"],
            [{"path": ".terraform.lock.hcl", "line": 2, "version": "0.0.25"}],
        )
        self.assertEqual(summary["backends"], [{"path": "versions.tf", "line": 13, "type": "s3"}])
        self.assertEqual(
            summary["required_versions"],
            [
                {"path": "versions.tf", "line": 4, "constraint": ">= 1.0"},
                {"path": "modules/legacy/main.tf", "line": 2, "constraint": ">= 1.5.6"},
            ],
        )
        self.assertEqual(self.results["v1"][2]["summary"]["backends"][0]["type"], "cloud")

    def test_a_file_that_does_not_parse_is_a_warning_not_a_crash(self) -> None:
        _, code, document = self.results["broken"]
        self.assertEqual(code, 2)
        self.assertFalse(document["complete"])
        self.assertEqual(len(document["warnings"]), 1, document["warnings"])
        warning = document["warnings"][0]
        self.assertEqual((warning["path"], warning["line"]), ("broken.tf", 1))
        self.assertIn("not closed", warning["message"])
        self.assert_markers("broken")


class ScannerCommandTest(unittest.TestCase):
    """The command line: output shapes, exit codes, and that it writes nothing."""

    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.dir = Path(self.temp.name)

    def test_json_output_shape(self) -> None:
        code, document = scan_json(materialize("v0", self.dir))
        self.assertEqual(code, 0)
        self.assertEqual(set(document), {"root", "complete", "findings", "warnings", "summary"})
        self.assertIsInstance(document["root"], str)
        self.assertIs(document["complete"], True)
        self.assertGreater(len(document["findings"]), 20)
        for finding in document["findings"]:
            self.assertEqual(set(finding), {"path", "line", "id", "safety", "message"})
            self.assertIsInstance(finding["path"], str)
            self.assertIsInstance(finding["line"], int)
            self.assertGreater(finding["line"], 0)
            self.assertIn(finding["id"], scan.RULES)
            self.assertIn(finding["safety"], SAFETIES)
            self.assertTrue(finding["message"])
        summary = document["summary"]
        self.assertEqual(
            set(summary),
            {
                "files_scanned",
                "counts",
                "ids",
                "locked_versions",
                "backends",
                "required_versions",
            },
        )
        self.assertEqual(summary["files_scanned"], 6)
        self.assertEqual(set(summary["counts"]), set(SAFETIES))
        self.assertEqual(sum(summary["counts"].values()), len(document["findings"]))
        self.assertEqual(set(summary["ids"]), {f["id"] for f in document["findings"]})

    def test_text_output_is_one_line_per_finding(self) -> None:
        root = materialize("v0", self.dir)
        _, document = scan_json(root)
        result = run_scanner(str(root))
        self.assertEqual(result.returncode, 0)
        expected = [
            f"{f['path']}:{f['line']} {f['id']} {f['safety']} {f['message']}"
            for f in document["findings"]
        ]
        self.assertEqual(result.stdout.splitlines(), expected)
        self.assertIn("NEEDS-REVIEW", result.stderr)

    def test_a_missing_path_exits_2(self) -> None:
        result = run_scanner(str(self.dir / "missing"))
        self.assertEqual(result.returncode, 2)
        self.assertIn("does not exist", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_a_directory_without_terraform_files_exits_2(self) -> None:
        (self.dir / "README.md").write_text("nothing here\n", encoding="utf-8")
        result = run_scanner(str(self.dir))
        self.assertEqual(result.returncode, 2)
        self.assertIn("no .tf files", result.stderr)

    def test_a_configuration_without_the_provider_exits_2(self) -> None:
        (self.dir / "main.tf").write_text('resource "null_resource" "x" {}\n', encoding="utf-8")
        result = run_scanner(str(self.dir))
        self.assertEqual(result.returncode, 2)
        self.assertIn("permitio/permit-io", result.stderr)

    def test_shapes_the_schema_does_not_allow_do_not_crash(self) -> None:
        (self.dir / "main.tf").write_text(
            "terraform {\n"
            "  backend {}\n"
            "  required_providers {\n"
            '    permitio = { source = "permitio/permit-io", version = "~> 1.0" }\n'
            "  }\n"
            "}\n"
            'resource "permitio_resource" "odd" {\n'
            '  key        = "odd"\n'
            '  name       = "Odd"\n'
            "  attributes = [1, 2]\n"
            "}\n",
            encoding="utf-8",
        )
        result = run_scanner(str(self.dir), "--json")
        self.assertNotIn("Traceback", result.stderr)
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads(result.stdout)
        self.assertEqual(document["warnings"], [])
        self.assertEqual(
            document["summary"]["backends"], [{"path": "main.tf", "line": 2, "type": ""}]
        )

    def test_a_check_that_fails_is_a_warning_and_exit_2(self) -> None:
        root = materialize("v1", self.dir)
        failure = TypeError("an unexpected shape")
        output = io.StringIO()
        patch = mock.patch.object(scan.Scan, "_check_object", side_effect=failure)
        with patch, contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
            code = scan.main([str(root), "--json"])
        self.assertEqual(code, 2)
        document = json.loads(output.getvalue())
        self.assertFalse(document["complete"])
        self.assertTrue(document["warnings"])
        for warning in document["warnings"]:
            self.assertIn("TypeError: an unexpected shape", warning["message"])
            self.assertGreater(warning["line"], 0)
        self.assertIn(("main.tf", 1), {(w["path"], w["line"]) for w in document["warnings"]})

    def test_tf_json_is_named_for_review_by_hand(self) -> None:
        root = materialize("v1", self.dir)
        (root / "extra.tf.json").write_text(
            '{"resource": {"permitio_tenant": {"x": {"key": "x", "name": "X"}}}}\n',
            encoding="utf-8",
        )
        code, document = scan_json(root)
        self.assertEqual(code, 2)
        self.assertEqual([w["path"] for w in document["warnings"]], ["extra.tf.json"])

    def test_the_scanner_writes_nothing(self) -> None:
        root = materialize("v0", self.dir)

        def snapshot() -> dict[str, tuple[bytes, int]]:
            return {
                path.relative_to(self.dir).as_posix(): (
                    path.read_bytes(),
                    path.stat().st_mtime_ns,
                )
                for path in sorted(self.dir.rglob("*"))
                if path.is_file()
            }

        before = snapshot()
        self.assertEqual(run_scanner(str(root)).returncode, 0)
        self.assertEqual(run_scanner(str(root), "--json").returncode, 0)
        self.assertEqual(snapshot(), before)


class ReferenceSyncTest(unittest.TestCase):
    """references/changes.md matches the guide's entries and the scanner's rules."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.entries = guide_entries()
        cls.sections = changes_sections()
        cls.impacts = guide_plan_impacts()

    def test_the_guide_has_the_expected_number_of_entries(self) -> None:
        self.assertGreaterEqual(len(self.entries), 47, sorted(self.entries))

    def test_every_guide_entry_has_a_section_and_back(self) -> None:
        missing = sorted(set(self.entries) - set(self.sections))
        extra = sorted(set(self.sections) - set(self.entries))
        self.assertEqual(missing, [], "guide entries with no section in changes.md")
        self.assertEqual(extra, [], "changes.md sections with no guide entry")

    def test_the_slug_matches_the_guides_own_links(self) -> None:
        anchors = set(self.entries.values())
        links = set(re.findall(r"\]\(#([^)]+)\)", GUIDE.read_text(encoding="utf-8")))
        self.assertTrue(links)
        self.assertEqual(sorted(links - anchors), [], "guide links the slug can't produce")

    def test_every_section_links_its_guide_entry(self) -> None:
        for entry, section in self.sections.items():
            with self.subTest(entry=entry):
                self.assertIn(f"({GUIDE_URL}#{self.entries.get(entry)})", section)

    def test_every_section_says_how_to_detect_edit_and_how_safe(self) -> None:
        for entry, section in self.sections.items():
            with self.subTest(entry=entry):
                for name in ("Detect", "Edit", "Safety", "Scanner"):
                    self.assertIsNotNone(field_line(section, name), f"no '- {name}:' line")
                safety_line = field_line(section, "Safety")
                assert safety_line is not None
                safety = re.findall(r"\b(SAFE|NEEDS-REVIEW)\b", safety_line)
                self.assertEqual(len(set(safety)), 1, "Safety must say SAFE or NEEDS-REVIEW")
                if not is_plain_none(self.impacts.get(entry)):
                    self.assertEqual(
                        safety[0],
                        "NEEDS-REVIEW",
                        f"plan impact {self.impacts.get(entry)!r} must be NEEDS-REVIEW",
                    )

    def test_scanner_lines_match_the_scanner_rules(self) -> None:
        for entry, section in self.sections.items():
            with self.subTest(entry=entry):
                scanner_line = field_line(section, "Scanner")
                safety_line = field_line(section, "Safety")
                self.assertIsNotNone(scanner_line, "no '- Scanner:' line")
                self.assertIsNotNone(safety_line, "no '- Safety:' line")
                assert scanner_line is not None and safety_line is not None
                listed = set(re.findall(r"\b(SAFE|NEEDS-REVIEW|INFO)\b", scanner_line))
                self.assertEqual(listed, set(scan.RULES.get(entry, ())))
                if "NEEDS-REVIEW" in safety_line:
                    self.assertNotIn("SAFE", listed)
        self.assertEqual(sorted(set(scan.RULES) - set(self.sections)), [])


class SkillDocumentTest(unittest.TestCase):
    """SKILL.md has the frontmatter an agent loads it by, and names its resources."""

    def test_frontmatter(self) -> None:
        text = SKILL_MD.read_text(encoding="utf-8")
        match = re.match(r"^---\n(.*?)\n---\n", text, re.S)
        self.assertIsNotNone(match, "SKILL.md has no frontmatter")
        assert match is not None
        fields = dict(re.findall(r"^(\w+): (.+)$", match.group(1), re.M))
        self.assertEqual(fields.get("name"), SKILL.name)
        description = fields.get("description", "")
        for trigger in ("permitio/permit-io", "1.0", "0.0.x", "OpenTofu", "replace"):
            self.assertIn(trigger, description)

    def test_frontmatter_meets_the_skill_format_limits(self) -> None:
        """The limits that skill validators and packagers enforce before loading a skill."""
        text = SKILL_MD.read_text(encoding="utf-8")
        match = re.match(r"^---\n(.*?)\n---\n", text, re.S)
        self.assertIsNotNone(match, "SKILL.md has no frontmatter")
        assert match is not None
        fields = dict(re.findall(r"^(\w+): (.+)$", match.group(1), re.M))
        self.assertEqual(set(fields), {"name", "description"})
        name, description = fields["name"], fields["description"]
        self.assertRegex(name, r"^[a-z0-9]+(-[a-z0-9]+)*$")
        self.assertLessEqual(len(name), 64)
        self.assertLessEqual(len(description), 1024)
        for character in ("<", ">"):
            self.assertNotIn(character, description)
        for plain_scalar_breaker in (": ", " #"):
            self.assertNotIn(plain_scalar_breaker, description, "not a plain YAML scalar")

    def test_rules_require_approval_and_the_preflight_stops_below_the_floor(self) -> None:
        text = SKILL_MD.read_text(encoding="utf-8")
        rules = re.search(r"^## Rules\n(.*?)^## ", text, re.S | re.M)
        self.assertIsNotNone(rules, "SKILL.md has no Rules section")
        assert rules is not None
        numbered = re.split(r"^\d+\. ", rules.group(1), flags=re.M)
        items = [" ".join(item.split()) for item in numbered]
        approval = [item for item in items if "without the user's explicit approval" in item]
        self.assertEqual(len(approval), 1, "no rule requires the user's explicit approval")
        for command in ("`terraform apply`", "`import`", "`state` subcommand", "`destroy`"):
            self.assertIn(command, approval[0])
        self.assertTrue(any(item.startswith("**Stop at any replacement**") for item in items))
        preflight = re.search(r"^## 1\. Preflight\n(.*?)^## ", text, re.S | re.M)
        self.assertIsNotNone(preflight, "SKILL.md has no preflight step")
        assert preflight is not None
        preflight_text = " ".join(preflight.group(1).split())
        self.assertIn(
            "**STOP if it is Terraform below 1.5.7 or OpenTofu below 1.11**", preflight_text
        )
        self.assertIn("don't continue on an older one even if asked", preflight_text)

    def test_names_the_floors_and_its_resources(self) -> None:
        text = SKILL_MD.read_text(encoding="utf-8")
        for needle in ("1.5.7", "1.11", "scripts/scan.py", "references/changes.md", GUIDE_URL):
            self.assertIn(needle, text)
        self.assertTrue(SCANNER.is_file())
        self.assertTrue(CHANGES.is_file())

    def test_relative_links_resolve(self) -> None:
        def anchors(path: Path) -> set[str]:
            text = path.read_text(encoding="utf-8")
            return {slug(h) for h in re.findall(r"^#+ (.+)$", text, re.M)}

        targets = {SKILL_MD: anchors(SKILL_MD), CHANGES: anchors(CHANGES)}
        checked = 0
        for document in (SKILL_MD, CHANGES):
            text = document.read_text(encoding="utf-8")
            for target, anchor in re.findall(r"\]\(((?:references/changes\.md)?)#([^)]+)\)", text):
                resolved = CHANGES if target else document
                checked += 1
                self.assertIn(anchor, targets[resolved], f"{document.name}: #{anchor}")
        self.assertGreater(checked, 5)


if __name__ == "__main__":
    unittest.main()
