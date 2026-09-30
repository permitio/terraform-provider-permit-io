#!/usr/bin/env python3
"""Read-only scanner for upgrading the permitio/permit-io Terraform provider to 1.0.

Walks a directory for Terraform configuration and reports each site that an entry
of the version 1 upgrade guide concerns, as ``path:line ID SAFETY message``. SAFETY
is SAFE (apply the edit), NEEDS-REVIEW (ask the user before any edit) or INFO (no
edit). references/changes.md describes every ID.

It never writes a file. It tokenizes HCL and follows its block structure, but it
evaluates no expression, reads no state and follows no module source outside the
scanned directory. SKILL.md lists what it can miss.

Exit status: 0 when the scan completed, whatever it found. 2 when it could not
scan, or not completely: a bad path, no Terraform file that uses the provider, a
file it could not read or parse, or a block with a shape its checks don't expect.
A warning names each such file or block, and the other findings are still
reported.

Standard library only, so it runs before anything is installed.
"""

from __future__ import annotations

import argparse
import bisect
import json
import os
import re
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path

SAFE = "SAFE"
REVIEW = "NEEDS-REVIEW"
INFO = "INFO"

# Every guide entry ID the scanner reports, with the safety values it reports it
# with. references/changes.md must list the same values in each entry's Scanner line.
RULES = {
    "P1": (SAFE, REVIEW),
    "C1": (REVIEW,),
    "C2": (REVIEW,),
    "S1": (INFO,),
    "S4": (REVIEW, INFO),
    "S5": (REVIEW,),
    "S6": (REVIEW,),
    "S7": (REVIEW,),
    "S8": (REVIEW,),
    "S11": (REVIEW,),
    "S12": (INFO,),
    "ST2": (REVIEW,),
    "ST3": (INFO,),
    "B4": (REVIEW, INFO),
    "B9": (INFO,),
    "B14": (REVIEW, INFO),
    "K1": (REVIEW,),
    "K5": (REVIEW, INFO),
    "K6": (REVIEW, INFO),
    "KP3": (INFO,),
}

TERRAFORM_FLOOR = (1, 5, 7)
OPENTOFU_FLOOR = (1, 11, 0)
TIMEOUT_MAX = 9223372036
SOURCE_RE = re.compile(
    r"^(?:(?:registry\.terraform\.io|registry\.opentofu\.org)/)?permitio/permit-io$", re.I
)
SKIP_DIRS = {".git", ".terraform", ".terragrunt-cache", "node_modules"}

# Resources whose key change plans a replacement in 1.0 (S1).
KEY_REPLACES = (
    "permitio_resource",
    "permitio_user_set",
    "permitio_resource_set",
    "permitio_proxy_config",
    "permitio_user_attribute",
)
# Data source arguments that 1.0 no longer requires (S12).
OPTIONAL_DATA_ARGS = {
    "permitio_resource": ("name", "actions"),
    "permitio_role": ("name",),
    "permitio_condition_set": ("name", "type", "conditions"),
}
# Arguments that name another object, which works reliably only with its key (K6).
OBJECT_ARGS = {
    "permitio_relation": ("subject_resource", "object_resource"),
    "permitio_resource_set": ("resource",),
    "permitio_role": ("resource", "extends", "permissions"),
    "permitio_role_derivation": ("resource", "on_resource", "role", "to_role", "linked_by"),
    "permitio_resource_instance": ("resource", "tenant"),
    "permitio_role_assignment": ("user", "role", "tenant"),
    "permitio_resource_instance_role_assignment": (
        "user",
        "role",
        "resource",
        "resource_instance",
        "tenant",
    ),
    "permitio_group_resource_instance_role_assignment": (
        "group",
        "role",
        "resource",
        "resource_instance",
        "tenant",
    ),
}
# JSON string arguments that 1.0 compares as JSON (B9).
JSON_ARGS = {
    "permitio_tenant": "attributes",
    "permitio_resource_instance": "attributes",
    "permitio_user_set": "conditions",
    "permitio_resource_set": "conditions",
}
# Import IDs that 1.0 splits on ":" and checks (ST2).
IMPORT_FORMATS = {
    "permitio_relation": "object_resource:key",
    "permitio_role_derivation": "resource:to_role:on_resource:role:linked_by",
    "permitio_role_assignment": "user:role:tenant",
    "permitio_resource_instance_role_assignment": "user:role:resource:resource_instance:tenant",
    "permitio_group_resource_instance_role_assignment": (
        "group:role:resource:resource_instance:tenant"
    ),
}
UNSUPPORTED_TYPES = ("object", "object_array")
PLACEHOLDER_RE = re.compile(r"your|<|>|xxx|change.?me|replace|placeholder|example|^$", re.I)
UUID_RE = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", re.I)
ID_REFERENCE_RE = re.compile(r"(?<![\w.])(?:data\.)?permitio_\w+\.[\w-]+(?:\[[^\]]*\])*\.id\b")
URL_PLACEHOLDER_RE = re.compile(r"(?<![$%])\{([^\W\d]\w*)\}")
PROXY_REFERENCE_RE = re.compile(r"(?<![\w.])permitio_proxy_config\.[\w-]+((?:\[[^\]]*\])*)(\.\w+)?")
CONSTRAINT_RE = re.compile(
    r"^\s*(=|!=|>=|<=|>|<|~>)?\s*v?(\d+(?:\.\d+){0,2})(?:-[0-9A-Za-z.-]+)?\s*$"
)
PIN_RE = re.compile(r"^[v~^=]*(\d+(?:\.\d+){0,2})$")
CI_PIN_RES = (
    (re.compile(r"\bterraform_version\s*:\s*['\"]?([^\s'\"#]+)"), "C1"),
    (re.compile(r"\btofu_version\s*:\s*['\"]?([^\s'\"#]+)"), "C2"),
    (re.compile(r"\bhashicorp/terraform:([^\s'\"#]+)"), "C1"),
    (re.compile(r"\bopentofu/opentofu:([^\s'\"#]+)"), "C2"),
)


class ParseError(Exception):
    """A construct the scanner relies on could not be read."""

    def __init__(self, line: int, message: str) -> None:
        super().__init__(message)
        self.line = line


@dataclass
class Token:
    kind: str
    text: str
    line: int
    literal: bool = True


@dataclass
class Attribute:
    name: str
    line: int
    tokens: list[Token]


@dataclass
class Block:
    type: str
    labels: list[str]
    line: int
    attributes: dict[str, Attribute] = field(default_factory=dict)
    blocks: list[Block] = field(default_factory=list)

    def children(self, block_type: str) -> list[Block]:
        return [child for child in self.blocks if child.type == block_type]


@dataclass
class Value:
    kind: str
    line: int
    text: str
    items: list = field(default_factory=list)

    def item(self, key: str) -> Value | None:
        for name, _, value in self.items if self.kind == "object" else ():
            if name == key:
                return value
        return None


@dataclass
class Finding:
    path: str
    line: int
    id: str
    safety: str
    message: str


# ---------------------------------------------------------------------------
# HCL tokenizer and block parser.

IDENT_RE = re.compile(r"[^\W\d][\w-]*")
NUMBER_RE = re.compile(r"\d+(?:\.\d+)?(?:[eE][+-]?\d+)?")
HEREDOC_RE = re.compile(r"<<-?([^\W\d][\w-]*)[ \t]*\r?\n")
PUNCTUATION = ("...", "==", "!=", "<=", ">=", "=>", "&&", "||")
OPENING = ("(", "[", "{")
CLOSING = (")", "]", "}")


def tokenize(text: str) -> list[Token]:
    """Splits HCL source into tokens, dropping comments and keeping newlines.

    Args:
        text: The file's content.

    Returns:
        The tokens, each with its 1-based line.

    Raises:
        ParseError: A string, heredoc, comment or interpolation is not closed.
    """
    starts = [0] + [match.end() for match in re.finditer(r"\n", text)]

    def line_at(index: int) -> int:
        return bisect.bisect_right(starts, index)

    tokens = []
    index = 0
    while index < len(text):
        char = text[index]
        heredoc = HEREDOC_RE.match(text, index) if char == "<" else None
        if char == "\n":
            tokens.append(Token("newline", "\n", line_at(index)))
            index += 1
        elif char in " \t\r":
            index += 1
        elif char == "#" or text.startswith("//", index):
            end = text.find("\n", index)
            index = len(text) if end < 0 else end
        elif text.startswith("/*", index):
            end = text.find("*/", index + 2)
            if end < 0:
                raise ParseError(line_at(index), "a /* comment is not closed")
            index = end + 2
        elif char == '"':
            end, literal = _scan_string(text, index, line_at)
            tokens.append(Token("string", text[index + 1 : end - 1], line_at(index), literal))
            index = end
        elif heredoc:
            body_end, end = _scan_heredoc(text, heredoc, line_at)
            body = text[heredoc.end() : body_end]
            tokens.append(Token("heredoc", body, line_at(index), literal=False))
            index = end
        else:
            index = _scan_word(text, index, tokens, line_at(index))
    return tokens


def _scan_word(text: str, index: int, tokens: list[Token], line: int) -> int:
    for kind, pattern in (("ident", IDENT_RE), ("number", NUMBER_RE)):
        match = pattern.match(text, index)
        if match:
            tokens.append(Token(kind, match.group(), line))
            return match.end()
    width = next((len(p) for p in PUNCTUATION if text.startswith(p, index)), 1)
    tokens.append(Token("punct", text[index : index + width], line))
    return index + width


def _scan_string(text: str, start: int, line_at) -> tuple[int, bool]:
    index = start + 1
    literal = True
    while index < len(text):
        char = text[index]
        if char == "\\":
            index += 2
        elif char == '"':
            return index + 1, literal
        elif char == "\n":
            break
        elif text.startswith(("$${", "%%{"), index):
            index += 3
        elif text.startswith(("${", "%{"), index):
            literal = False
            index = _scan_template(text, index + 2, line_at)
        else:
            index += 1
    raise ParseError(line_at(start), "a quoted string is not closed on its line")


def _scan_template(text: str, index: int, line_at) -> int:
    start = index
    depth = 1
    while index < len(text):
        char = text[index]
        if char == '"':
            index, _ = _scan_string(text, index, line_at)
            continue
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return index + 1
        index += 1
    raise ParseError(line_at(start), "a ${ or %{ template sequence is not closed")


def _scan_heredoc(text: str, match: re.Match, line_at) -> tuple[int, int]:
    marker = match.group(1)
    position = match.end()
    while position < len(text):
        newline = text.find("\n", position)
        line_end = len(text) if newline < 0 else newline
        if text[position:line_end].strip() == marker:
            return position, line_end
        position = line_end + 1
    raise ParseError(line_at(match.start()), f"the heredoc <<{marker} is not closed")


def parse(tokens: list[Token]) -> Block:
    """Parses tokens into a tree of blocks and arguments.

    Args:
        tokens: The output of tokenize.

    Returns:
        A root block that holds the file's top-level blocks and arguments.

    Raises:
        ParseError: The block structure is unbalanced or not HCL.
    """
    root = Block("", [], 0)
    index, closed = _parse_body(tokens, 0, root)
    if closed:
        raise ParseError(tokens[index - 1].line, "a } closes no block")
    return root


def _parse_body(tokens: list[Token], index: int, block: Block) -> tuple[int, bool]:
    while index < len(tokens):
        token = tokens[index]
        if token.kind == "newline":
            index += 1
            continue
        if token.kind == "punct" and token.text == "}":
            return index + 1, True
        if token.kind != "ident":
            raise ParseError(token.line, f"expected an argument or a block, found {token.text!r}")
        following = tokens[index + 1] if index + 1 < len(tokens) else None
        if following is not None and following.kind == "punct" and following.text == "=":
            end = _expression_end(tokens, index + 2)
            block.attributes[token.text] = Attribute(
                token.text, token.line, tokens[index + 2 : end]
            )
            index = end
            continue
        labels = []
        index += 1
        while index < len(tokens) and tokens[index].kind in ("string", "ident"):
            labels.append(tokens[index].text)
            index += 1
        if index >= len(tokens) or tokens[index].kind != "punct" or tokens[index].text != "{":
            raise ParseError(token.line, f"expected a block body after {token.text!r}")
        child = Block(token.text, labels, token.line)
        index, closed = _parse_body(tokens, index + 1, child)
        if not closed:
            raise ParseError(token.line, f"the {token.text} block is not closed")
        block.blocks.append(child)
    return index, False


def _expression_end(tokens: list[Token], index: int) -> int:
    start = tokens[index - 1].line
    depth = 0
    while index < len(tokens):
        token = tokens[index]
        if token.kind == "punct" and token.text in OPENING:
            depth += 1
        elif token.kind == "punct" and token.text in CLOSING:
            if depth == 0:
                return index
            depth -= 1
        elif token.kind == "newline" and depth == 0:
            return index
        index += 1
    if depth:
        raise ParseError(start, "an expression is not closed")
    return index


def value_of(tokens: list[Token]) -> Value:
    """Reads an argument's value: a literal, an object or tuple, or an expression.

    Args:
        tokens: The value's tokens.

    Returns:
        The value. Object and tuple items are values too; anything else that is not
        a single literal is an expression, kept as text.
    """
    while tokens and tokens[0].kind == "newline":
        tokens = tokens[1:]
    while tokens and tokens[-1].kind == "newline":
        tokens = tokens[:-1]
    if not tokens:
        return Value("expression", 0, "")
    first = tokens[0]
    text = _join(tokens)
    if len(tokens) == 1:
        kind = _literal_kind(first)
        if kind:
            return Value(kind, first.line, first.text)
    if len(tokens) == 2 and first.text == "-" and tokens[1].kind == "number":
        return Value("number", first.line, "-" + tokens[1].text)
    inner = [token for token in tokens[1:-1] if token.kind != "newline"]
    is_for = bool(inner) and inner[0].kind == "ident" and inner[0].text == "for"
    if first.text in ("{", "[") and _closing(tokens) == len(tokens) - 1 and not is_for:
        if first.text == "{":
            return Value("object", first.line, text, _object_items(tokens[1:-1]))
        items = [value_of(part) for part in _split(tokens[1:-1], (",",))]
        return Value("tuple", first.line, text, items)
    return Value("expression", first.line, text)


def _literal_kind(token: Token) -> str | None:
    if token.kind == "string":
        return "string" if token.literal else "template"
    if token.kind in ("heredoc", "number"):
        return token.kind
    if token.kind == "ident" and token.text in ("true", "false"):
        return "bool"
    if token.kind == "ident" and token.text == "null":
        return "null"
    return None


def _closing(tokens: list[Token]) -> int:
    depth = 0
    for index, token in enumerate(tokens):
        if token.kind == "punct" and token.text in OPENING:
            depth += 1
        elif token.kind == "punct" and token.text in CLOSING:
            depth -= 1
            if depth == 0:
                return index
    return -1


def _split(tokens: list[Token], separators: tuple[str, ...]) -> list[list[Token]]:
    parts = [[]]
    depth = 0
    for token in tokens:
        if token.kind == "punct" and token.text in OPENING:
            depth += 1
        elif token.kind == "punct" and token.text in CLOSING:
            depth -= 1
        if depth == 0 and (token.text in separators and token.kind in ("punct", "newline")):
            parts.append([])
            continue
        parts[-1].append(token)
    return [part for part in parts if any(token.kind != "newline" for token in part)]


def _object_items(tokens: list[Token]) -> list[tuple[str, int, Value]]:
    items = []
    for part in _split(tokens, (",", "\n")):
        equals = next(
            (
                index
                for index, token in enumerate(part)
                if token.kind == "punct" and token.text in ("=", ":")
            ),
            None,
        )
        if equals is None:
            continue
        key_tokens = [token for token in part[:equals] if token.kind != "newline"]
        if len(key_tokens) == 1 and key_tokens[0].kind in ("ident", "string"):
            key = key_tokens[0].text
        else:
            key = _join(key_tokens)
        line = key_tokens[0].line if key_tokens else part[equals].line
        items.append((key, line, value_of(part[equals + 1 :])))
    return items


def _join(tokens: list[Token]) -> str:
    pieces = []
    previous = None
    for token in tokens:
        if token.kind == "newline":
            continue
        word = token.kind in ("ident", "number", "string")
        if word and previous in ("ident", "number", "string"):
            pieces.append(" ")
        pieces.append(f'"{token.text}"' if token.kind == "string" else token.text)
        previous = token.kind
    return "".join(pieces)


# ---------------------------------------------------------------------------
# Versions and constraints.


def parse_constraint(text: str) -> list[tuple[str, tuple[int, ...]]] | None:
    """Parses a Terraform version constraint such as ``">= 1.0, < 2.0"``.

    Returns:
        The clauses as (operator, version numbers), or None when a clause is not a
        version constraint.
    """
    clauses = []
    for piece in text.split(","):
        match = CONSTRAINT_RE.match(piece)
        if not match:
            return None
        clauses.append((match.group(1) or "=", tuple(int(n) for n in match.group(2).split("."))))
    return clauses


def satisfies(version: tuple[int, int, int], clauses: list[tuple[str, tuple[int, ...]]]) -> bool:
    """Reports whether a version meets every clause of a constraint."""
    for operator, numbers in clauses:
        bound = numbers + (0,) * (3 - len(numbers))
        if operator == "~>":
            if len(numbers) == 1:
                upper = (numbers[0] + 1, 0, 0)
            else:
                upper = numbers[:-2] + (numbers[-2] + 1,) + (0,) * (4 - len(numbers))
            if not bound <= version < upper:
                return False
        elif not {
            "=": version == bound,
            "!=": version != bound,
            ">": version > bound,
            ">=": version >= bound,
            "<": version < bound,
            "<=": version <= bound,
        }[operator]:
            return False
    return True


def allows_below(clauses: list[tuple[str, tuple[int, ...]]], floor: tuple[int, int, int]) -> bool:
    """Reports whether a constraint allows any 0.x or 1.x version below floor."""
    probes = [
        (major, minor, patch) for major in (0, 1) for minor in range(20) for patch in range(41)
    ]
    return any(probe < floor and satisfies(probe, clauses) for probe in probes)


def parse_pin(text: str) -> tuple[int, ...] | None:
    """Reads a pinned CLI version; a partial one, such as 1.4, stands for its latest patch."""
    match = PIN_RE.match(text.strip().lower())
    if not match:
        return None
    numbers = tuple(int(n) for n in match.group(1).split("."))
    return numbers + (10**6,) * (3 - len(numbers))


# ---------------------------------------------------------------------------
# The scan.


class Scan:
    """Scans one directory or file and collects findings, warnings and a summary."""

    def __init__(self, root: Path) -> None:
        self.root = root
        self.findings: list[Finding] = []
        self.warnings: list[dict] = []
        self.complete = True
        self.files = 0
        self.uses_provider = False
        self.modules: dict[str, list[tuple[str, Block]]] = {}
        self.locked: list[dict] = []
        self.backends: list[dict] = []
        self.required_versions: list[dict] = []

    def report(self, path: str, line: int, entry: str, safety: str, message: str) -> None:
        self.findings.append(Finding(path, line, entry, safety, message))

    def warn(self, path: str, line: int, message: str) -> None:
        """Records a file or path the scan could not cover, which makes it incomplete."""
        self.warnings.append({"path": path, "line": line, "message": message})
        self.complete = False

    def run(self) -> None:
        if not self.root.exists():
            self.warn(str(self.root), 0, "the path does not exist")
            return
        for path in self._walk():
            self._scan_file(path)
        if self.files == 0:
            self.warn(str(self.root), 0, "no .tf files found")
            return
        for directory in sorted(self.modules):
            self._check_module(self.modules[directory])
        if not self.uses_provider:
            self.warn(str(self.root), 0, "no .tf file uses the permitio/permit-io provider")
        self.findings.sort(key=lambda f: (f.path, f.line, f.id, f.safety, f.message))

    def _walk(self) -> list[Path]:
        if self.root.is_file():
            return [self.root]
        paths = []
        for directory, subdirs, names in os.walk(self.root):
            subdirs[:] = sorted(name for name in subdirs if name not in SKIP_DIRS)
            paths.extend(Path(directory) / name for name in sorted(names))
        return paths

    def _relative(self, path: Path) -> str:
        if self.root.is_file():
            return path.name
        return path.relative_to(self.root).as_posix()

    def _read(self, path: Path, relative: str) -> str | None:
        try:
            return path.read_text(encoding="utf-8-sig")
        except (OSError, UnicodeDecodeError) as error:
            self.warn(relative, 0, f"could not read the file: {error}; review it by hand")
            return None

    def _scan_file(self, path: Path) -> None:
        relative = self._relative(path)
        name = path.name
        if name.endswith(".tf.json"):
            text = self._read(path, relative)
            if text is not None and "permitio" in text:
                self.warn(relative, 0, "the scanner does not read .tf.json; review it by hand")
            return
        if name.endswith(".tf"):
            self._scan_hcl(path, relative)
        elif name == ".terraform.lock.hcl":
            self._scan_lock(path, relative)
        elif name in (".terraform-version", ".opentofu-version", ".tool-versions"):
            self._scan_version_file(path, relative)
        elif _is_ci_file(relative):
            self._scan_ci_file(path, relative)

    def _parse(self, path: Path, relative: str) -> Block | None:
        text = self._read(path, relative)
        if text is None:
            return None
        try:
            return parse(tokenize(text))
        except ParseError as error:
            self.warn(relative, error.line, f"could not parse: {error}; review this file by hand")
            return None

    def _scan_hcl(self, path: Path, relative: str) -> None:
        self.files += 1
        root = self._parse(path, relative)
        if root is None:
            return
        directory = str(Path(relative).parent)
        self.modules.setdefault(directory, []).extend((relative, block) for block in root.blocks)

    def _scan_lock(self, path: Path, relative: str) -> None:
        root = self._parse(path, relative)
        for block in root.children("provider") if root else ():
            version = _literal(block, "version")
            if block.labels and SOURCE_RE.match(block.labels[0]) and version is not None:
                self.locked.append({"path": relative, "line": block.line, "version": version})

    def _scan_version_file(self, path: Path, relative: str) -> None:
        text = self._read(path, relative)
        for number, raw in enumerate((text or "").splitlines(), start=1):
            words = raw.split("#", 1)[0].split()
            if path.name == ".tool-versions":
                pairs = [(words[0], words[1])] if len(words) >= 2 else []
            else:
                cli = "opentofu" if path.name == ".opentofu-version" else "terraform"
                pairs = [(cli, words[0])] if words else []
            for cli, version in pairs:
                if cli in ("terraform", "opentofu"):
                    entry = "C2" if cli == "opentofu" else "C1"
                    self._check_pin(relative, number, entry, version)

    def _scan_ci_file(self, path: Path, relative: str) -> None:
        text = self._read(path, relative)
        for number, raw in enumerate((text or "").splitlines(), start=1):
            for pattern, entry in CI_PIN_RES:
                for match in pattern.finditer(raw):
                    self._check_pin(relative, number, entry, match.group(1))

    def _check_pin(self, relative: str, line: int, entry: str, version: str) -> None:
        pinned = parse_pin(version)
        if entry == "C2":
            floor, floor_name, cli = OPENTOFU_FLOOR, "1.11", "OpenTofu"
        else:
            floor, floor_name, cli = TERRAFORM_FLOOR, "1.5.7", "Terraform"
        if pinned is not None and pinned < floor:
            self.report(
                relative,
                line,
                entry,
                REVIEW,
                f"pins {cli} {version}, below the {floor_name} that 1.0 supports: "
                "raise it, or stay on 0.0.x",
            )

    # -- Rules on one module (directory) ------------------------------------

    def _guarded(self, check, path: str, block: Block, *arguments):
        """Runs one block's check; a construct it doesn't expect becomes a warning.

        The rules assume the shapes the provider's schema allows. A block that parses
        but has another shape must not stop the scan with a traceback: the warning
        names it for review by hand, and the scan exits 2 as incomplete.
        """
        try:
            return check(path, block, *arguments)
        except Exception as error:  # noqa: BLE001 - any failure is reported, not hidden
            self.warn(
                path,
                block.line,
                f"could not check this {block.type} block ({type(error).__name__}: "
                f"{error}); review it by hand",
            )
            return None

    def _check_module(self, entries: list[tuple[str, Block]]) -> None:
        index = {}
        keys = set()
        for path, block in entries:
            if block.type in ("resource", "data") and len(block.labels) >= 2:
                index[(block.type, block.labels[0], block.labels[1])] = (path, block)
                key = _literal(block, "key")
                if block.labels[0].startswith("permitio_") and key is not None:
                    keys.add(key)
        local_names = set()
        declared = False
        for path, block in entries:
            if block.type == "terraform":
                declared |= bool(self._guarded(self._check_terraform, path, block, local_names))
        providers = local_names or {"permitio"}
        objects = [
            (path, block)
            for path, block in entries
            if block.type in ("resource", "data")
            and len(block.labels) >= 2
            and block.labels[0].startswith("permitio_")
        ]
        for path, block in entries:
            if block.type == "provider" and block.labels[:1] and block.labels[0] in providers:
                self.uses_provider = True
                self._guarded(self._check_provider, path, block)
            elif block.type == "output" and block.labels:
                self._guarded(self._check_output, path, block)
            elif block.type == "import":
                self._guarded(self._check_import, path, block)
        for path, block in objects:
            self._guarded(self._check_object, path, block, index, keys)
        self.uses_provider |= bool(objects) or declared
        if objects and not declared:
            path, block = objects[0]
            self.report(
                path,
                block.line,
                "P1",
                REVIEW,
                "this module uses permitio_ objects but no required_providers entry has "
                'source "permitio/permit-io"; check where the version is constrained',
            )
        user_sets = [
            (path, block)
            for path, block in entries
            if block.type == "resource" and block.labels[:1] == ["permitio_user_set"]
        ]
        if user_sets and not any("resource" in block.attributes for _, block in user_sets):
            path, block = user_sets[0]
            self.report(
                path,
                block.line,
                "S4",
                INFO,
                "user sets in a state written by 0.0.x: run terraform apply -refresh-only "
                "once after the upgrade, only with the user's approval",
            )

    def _check_terraform(self, path: str, block: Block, local_names: set[str]) -> bool:
        declared = False
        required = block.attributes.get("required_version")
        if required is not None:
            self._check_required_version(path, required)
        for backend in block.children("backend"):
            backend_type = backend.labels[0] if backend.labels else ""
            self.backends.append({"path": path, "line": backend.line, "type": backend_type})
        for cloud in block.children("cloud"):
            self.backends.append({"path": path, "line": cloud.line, "type": "cloud"})
        for providers in block.children("required_providers"):
            for name, attribute in providers.attributes.items():
                value = value_of(attribute.tokens)
                source = value.item("source")
                if source is None or source.kind != "string" or not SOURCE_RE.match(source.text):
                    continue
                declared = True
                local_names.add(name)
                self._check_constraint(path, attribute.line, value.item("version"))
                if name != "permitio":
                    self.report(
                        path,
                        attribute.line,
                        "KP3",
                        INFO,
                        f'local name "{name}" for permitio/permit-io: every permitio_ object '
                        f"needs provider = {name}; the guide uses permitio",
                    )
        return declared

    def _check_required_version(self, path: str, attribute: Attribute) -> None:
        value = value_of(attribute.tokens)
        if value.kind != "string":
            return
        self.required_versions.append(
            {"path": path, "line": attribute.line, "constraint": value.text}
        )
        clauses = parse_constraint(value.text)
        if clauses is not None and allows_below(clauses, TERRAFORM_FLOOR):
            self.report(
                path,
                attribute.line,
                "C1",
                REVIEW,
                f'required_version "{value.text}" allows Terraform below 1.5.7: raise it to '
                '">= 1.5.7" once every place the configuration runs is on 1.5.7 or later',
            )

    def _check_constraint(self, path: str, line: int, version: Value | None) -> None:
        if version is None:
            self.report(path, line, "P1", SAFE, 'no version constraint: add version = "~> 1.0"')
            return
        clauses = parse_constraint(version.text) if version.kind == "string" else None
        if clauses is None:
            self.report(
                path,
                version.line,
                "P1",
                REVIEW,
                f"could not read the version constraint {version.text}",
            )
        elif not satisfies((1, 0, 0), clauses):
            self.report(
                path,
                version.line,
                "P1",
                SAFE,
                f'version "{version.text}" never selects 1.0: set version = "~> 1.0"',
            )
        elif satisfies((2, 0, 0), clauses):
            self.report(
                path,
                version.line,
                "P1",
                SAFE,
                f'version "{version.text}" also allows 2.0 and later: set version = "~> 1.0"',
            )

    def _check_provider(self, path: str, block: Block) -> None:
        attributes = block.attributes
        if "version" in attributes:
            self.report(
                path,
                attributes["version"].line,
                "P1",
                SAFE,
                "a version in the provider block: remove it, and constrain the version to "
                '"~> 1.0" in required_providers',
            )
        api_key = _literal(block, "api_key")
        if api_key is not None:
            if PLACEHOLDER_RE.search(api_key):
                message = (
                    "api_key looks like a placeholder, and in 1.0 the provider block wins "
                    "over PERMITIO_API_KEY, so the plan fails: remove the argument"
                )
            else:
                message = (
                    "api_key is written in the configuration, and in 1.0 the provider block "
                    "wins over PERMITIO_API_KEY: take it from a sensitive variable or the "
                    "environment instead"
                )
            self.report(path, attributes["api_key"].line, "B4", REVIEW, message)
        api_url = _literal(block, "api_url")
        if api_url is not None and re.match(r"^https?://[^/\s]+", api_url):
            self.report(
                path,
                attributes["api_url"].line,
                "B4",
                INFO,
                "api_url in the provider block wins over PERMITIO_API_URL in 1.0",
            )
        elif api_url is not None:
            self.report(
                path,
                attributes["api_url"].line,
                "S11",
                REVIEW,
                "api_url is not an absolute http or https URL, which fails the plan in 1.0",
            )
        timeout = attributes.get("timeout")
        seconds = value_of(timeout.tokens) if timeout else None
        if (
            timeout is not None
            and seconds is not None
            and seconds.kind == "number"
            and not 1 <= float(seconds.text) <= TIMEOUT_MAX
        ):
            self.report(
                path,
                timeout.line,
                "S11",
                REVIEW,
                f"timeout {seconds.text} fails the plan in 1.0, which takes 1 to "
                f"{TIMEOUT_MAX} seconds; 0 or less used to mean no limit",
            )

    def _check_object(self, path: str, block: Block, index: dict, keys: set) -> None:
        kind = block.labels[0]
        attributes = block.attributes
        if "updated_at" in attributes:
            self.report(
                path,
                attributes["updated_at"].line,
                "S5",
                REVIEW,
                "updated_at is read-only in 1.0, so setting it fails the plan: delete it",
            )
        if block.type == "data":
            optional = [arg for arg in OPTIONAL_DATA_ARGS.get(kind, ()) if arg in attributes]
            if optional:
                self.report(
                    path,
                    attributes[optional[0]].line,
                    "S12",
                    INFO,
                    f"1.0 no longer requires {' and '.join(optional)} on this data source: "
                    f"optionally remove {'it' if len(optional) == 1 else 'them'}",
                )
            return
        key = attributes.get("key")
        if kind in KEY_REPLACES and key is not None and value_of(key.tokens).kind != "string":
            self.report(
                path,
                key.line,
                "S1",
                INFO,
                "key comes from an expression: when its value changes, 1.0 replaces the "
                "object, and Permit deletes what depends on it",
            )
        if kind == "permitio_user_set":
            self._check_user_set(path, block)
        if kind in JSON_ARGS and JSON_ARGS[kind] in attributes:
            self._check_json_argument(path, kind, attributes[JSON_ARGS[kind]])
        if kind == "permitio_resource" and "attributes" in attributes:
            value = value_of(attributes["attributes"].tokens)
            if value.kind == "object" and not value.items:
                self.report(
                    path,
                    value.line,
                    "ST3",
                    INFO,
                    "attributes = {}: after an import, the first plan adds it with one "
                    "in-place update that changes nothing in Permit",
                )
        if kind in ("permitio_resource", "permitio_user_attribute"):
            self._check_attribute_types(path, block)
        for argument in OBJECT_ARGS.get(kind, ()):
            if argument in attributes:
                self._check_names_by_key(path, attributes[argument], keys)
        if kind == "permitio_proxy_config":
            self._check_proxy_config(path, block, index)
        if kind == "permitio_role_derivation":
            self._check_role_derivation(path, block, index)

    def _check_user_set(self, path: str, block: Block) -> None:
        resource = block.attributes.get("resource")
        if resource is not None:
            self.report(
                path,
                resource.line,
                "S4",
                REVIEW,
                "permitio_user_set has no resource argument in 1.0, so the plan fails: delete "
                "it (it never had an effect), then run terraform apply -refresh-only once, "
                "only with the user's approval",
            )
        for lifecycle in block.children("lifecycle"):
            ignored = lifecycle.attributes.get("ignore_changes")
            if ignored is None:
                continue
            names = [token.text for token in ignored.tokens if token.kind == "ident"]
            if "resource" in names:
                self.report(
                    path,
                    ignored.line,
                    "S4",
                    REVIEW,
                    "ignore_changes names resource, which permitio_user_set no longer has, "
                    "so the plan fails: remove it from the list",
                )

    def _check_json_argument(self, path: str, kind: str, attribute: Attribute) -> None:
        value = value_of(attribute.tokens)
        if value.kind in ("string", "template", "heredoc"):
            self.report(
                path,
                attribute.line,
                "B9",
                INFO,
                f"{attribute.name} is written as a JSON string: 1.0 compares it as JSON, and "
                "a reformatted string plans one in-place update that sends the same object",
            )
        elif kind in ("permitio_tenant", "permitio_resource_instance") and re.fullmatch(
            r"jsonencode\(\{\}\)", value.text
        ):
            self.report(
                path,
                attribute.line,
                "ST3",
                INFO,
                "attributes = jsonencode({}): after an import, the first plan adds it with "
                "one in-place update that changes nothing in Permit",
            )

    def _check_attribute_types(self, path: str, block: Block) -> None:
        if block.labels[0] == "permitio_user_attribute":
            attribute = block.attributes.get("type")
            types = [value_of(attribute.tokens)] if attribute else []
        else:
            attribute = block.attributes.get("attributes")
            value = value_of(attribute.tokens) if attribute else Value("object", 0, "")
            definitions = [item for _, _, item in value.items] if value.kind == "object" else []
            types = [definition.item("type") for definition in definitions]
        for value in types:
            if value is not None and value.kind == "string" and value.text in UNSUPPORTED_TYPES:
                self.report(
                    path,
                    value.line,
                    "K1",
                    REVIEW,
                    f'attribute type "{value.text}" is not supported in 1.0, so the plan fails: '
                    "manage this object outside Terraform until it is",
                )

    def _check_names_by_key(self, path: str, attribute: Attribute, keys: set) -> None:
        argument = attribute.name
        value = value_of(attribute.tokens)
        if ID_REFERENCE_RE.search(value.text):
            self.report(
                path,
                attribute.line,
                "K6",
                REVIEW,
                f"{argument} names another object by ID, and Permit returns keys: use the key. "
                "Changing it can plan an update, or a replacement where the argument forces one",
            )
            return
        texts = [item.text for item in value.items] if value.kind == "tuple" else [value.text]
        if any(UUID_RE.match(text) and text not in keys for text in texts):
            self.report(
                path,
                attribute.line,
                "K6",
                INFO,
                f"{argument} is a UUID that no permitio_ object in this module has as its key: "
                "nothing to do if it is the object's key. If it is an ID, the plan shows a "
                "change for it on every run or the apply fails: then ask the user about "
                "using the key",
            )

    def _check_proxy_config(self, path: str, block: Block, index: dict) -> None:
        mechanism = block.attributes.get("auth_mechanism")
        if mechanism is not None:
            value = value_of(mechanism.tokens)
            if value.kind == "string" and value.text.lower() == "headers":
                self.report(
                    path,
                    mechanism.line,
                    "S7",
                    REVIEW,
                    'auth_mechanism "Headers" fails the plan in 1.0: use Bearer or Basic, or '
                    "manage this proxy config outside Terraform",
                )
        rules = block.attributes.get("mapping_rules")
        value = value_of(rules.tokens) if rules else None
        if value is None or value.kind != "tuple":
            return
        seen = {}
        for position, rule in enumerate(value.items):
            url, method = rule.item("url"), rule.item("http_method")
            if url is None:
                continue
            line = next(line for key, line, _ in rule.items if key == "url")
            if url.kind == "string" and method is not None and method.kind == "string":
                pair = (url.text, method.text)
                if pair in seen:
                    self.report(
                        path,
                        line,
                        "S8",
                        REVIEW,
                        f"mapping_rules[{position}] has the url and http_method of "
                        f"mapping_rules[{seen[pair]}], which fails the plan in 1.0: keep one "
                        "rule for each pair",
                    )
                else:
                    seen[pair] = position
            url_type = rule.item("url_type")
            if url_type is None or url_type.kind != "string" or url_type.text != "regex":
                self._check_url_placeholders(path, line, url, rule.item("resource"), index)

    def _check_url_placeholders(
        self, path: str, line: int, url: Value, resource: Value | None, index: dict
    ) -> None:
        if url.kind not in ("string", "template"):
            return
        target = self._resource_block(resource, index)
        if target is None:
            name = resource.text if resource is not None else "the rule's resource"
            declared = None
        else:
            name = _literal(target[1], "key") or f"permitio_resource.{target[1].labels[1]}"
            attributes = target[1].attributes.get("attributes")
            declared = value_of(attributes.tokens) if attributes else Value("object", 0, "")
        for placeholder in URL_PLACEHOLDER_RE.findall(url.text):
            added = f"the url placeholder {{{placeholder}}} makes Permit add an attribute "
            added += f"{placeholder} to {name}"
            if declared is None:
                self.report(
                    path,
                    line,
                    "K5",
                    INFO,
                    f"{added}: if a permitio_resource manages it, declare the attribute there",
                )
            elif (
                target is not None
                and declared.kind == "object"
                and declared.item(placeholder) is None
            ):
                self.report(
                    path,
                    line,
                    "K5",
                    REVIEW,
                    f"{added}, which permitio_resource.{target[1].labels[1]} ({target[0]}:"
                    f"{target[1].line}) doesn't declare, so its plans remove it: declare it, "
                    'or match the url with url_type = "regex"',
                )

    def _resource_block(self, value: Value | None, index: dict) -> tuple[str, Block] | None:
        if value is None:
            return None
        match = re.fullmatch(r"permitio_resource\.([\w-]+)\.(?:key|id)", value.text)
        if match:
            return index.get(("resource", "permitio_resource", match.group(1)))
        resources = [
            entry
            for (block_type, kind, _), entry in index.items()
            if block_type == "resource" and kind == "permitio_resource"
        ]
        if value.kind == "string":
            return next((e for e in resources if _literal(e[1], "key") == value.text), None)
        return None

    def _resource_key(self, value: Value | None, index: dict) -> str | None:
        if value is None:
            return None
        if value.kind == "string":
            return value.text
        match = re.fullmatch(r"(data\.)?permitio_resource\.([\w-]+)\.key", value.text)
        if not match:
            return None
        entry = index.get(
            ("data" if match.group(1) else "resource", "permitio_resource", match.group(2))
        )
        return _literal(entry[1], "key") if entry else None

    def _role_resource(self, value: Value | None, index: dict) -> str | None:
        if value is None:
            return None
        match = re.fullmatch(r"permitio_role\.([\w-]+)\.key", value.text)
        if match:
            entry = index.get(("resource", "permitio_role", match.group(1)))
            roles = [entry[1]] if entry else []
        elif value.kind == "string":
            roles = [
                block
                for (block_type, kind, _), (_, block) in index.items()
                if block_type == "resource"
                and kind == "permitio_role"
                and _literal(block, "key") == value.text
            ]
        else:
            return None
        resources = set()
        for role in roles:
            attribute = role.attributes.get("resource")
            resources.add(
                self._resource_key(value_of(attribute.tokens), index) if attribute else ""
            )
        return resources.pop() if len(resources) == 1 else None

    def _check_role_derivation(self, path: str, block: Block, index: dict) -> None:
        values = {name: value_of(a.tokens) for name, a in block.attributes.items()}
        resource = self._resource_key(values.get("resource"), index)
        on_resource = self._resource_key(values.get("on_resource"), index)
        role = self._role_resource(values.get("role"), index)
        to_role = self._role_resource(values.get("to_role"), index)
        if resource and on_resource and resource != on_resource and role and to_role:
            if (role, to_role) == (on_resource, resource):
                return
            reversed_roles = (role, to_role) == (resource, on_resource)
        else:
            reversed_roles = _names_look_reversed(values)
        if reversed_roles:
            self.report(
                path,
                block.line,
                "B14",
                REVIEW,
                "role and to_role look reversed: role must be a role on on_resource, and "
                "to_role a role on resource. Swapping them replaces the derivation",
            )
        else:
            self.report(
                path,
                block.line,
                "B14",
                INFO,
                "check that role is a role on on_resource and to_role a role on resource; "
                "the 0.0.25 docs and example reversed them",
            )

    def _check_output(self, path: str, block: Block) -> None:
        value = block.attributes.get("value")
        sensitive = block.attributes.get("sensitive")
        if value is None or "nonsensitive(" in _join(value.tokens):
            return
        if sensitive is not None and value_of(sensitive.tokens).text == "true":
            return
        for match in PROXY_REFERENCE_RE.finditer(_join(value.tokens)):
            attribute = match.group(2)
            if attribute is None or attribute == ".auth_secret":
                self.report(
                    path,
                    value.line,
                    "S6",
                    REVIEW,
                    f'output "{block.labels[0]}" holds auth_secret, which is sensitive in 1.0, '
                    "so the plan fails: add sensitive = true",
                )
                return

    def _check_import(self, path: str, block: Block) -> None:
        target = block.attributes.get("to")
        identifier = block.attributes.get("id")
        if target is None or identifier is None:
            return
        kind = _join(target.tokens).split(".", 1)[0]
        value = value_of(identifier.tokens)
        if kind not in IMPORT_FORMATS or value.kind != "string":
            return
        expected = IMPORT_FORMATS[kind]
        parts = value.text.split(":")
        if len(parts) != expected.count(":") + 1 or not all(parts):
            self.report(
                path,
                identifier.line,
                "ST2",
                REVIEW,
                f"1.0 fails this import: the ID must be {expected}, with no empty part",
            )

    # -- Output ------------------------------------------------------------

    def as_json(self) -> dict:
        counts = {safety: 0 for safety in (SAFE, REVIEW, INFO)}
        for finding in self.findings:
            counts[finding.safety] += 1
        return {
            "root": str(self.root),
            "complete": self.complete,
            "findings": [asdict(finding) for finding in self.findings],
            "warnings": self.warnings,
            "summary": {
                "files_scanned": self.files,
                "counts": counts,
                "ids": sorted({finding.id for finding in self.findings}, key=_id_order),
                "locked_versions": self.locked,
                "backends": self.backends,
                "required_versions": self.required_versions,
            },
        }


def _literal(block: Block, name: str) -> str | None:
    attribute = block.attributes.get(name)
    value = value_of(attribute.tokens) if attribute else None
    return value.text if value is not None and value.kind == "string" else None


def _names_look_reversed(values: dict[str, Value]) -> bool:
    texts = {}
    for name in ("resource", "on_resource", "role", "to_role"):
        value = values.get(name)
        if value is None or value.kind != "string":
            return False
        texts[name] = re.sub(r"[^a-z0-9]", "", value.text.lower())
    resource, on_resource = texts["resource"], texts["on_resource"]
    role, to_role = texts["role"], texts["to_role"]
    if not resource or not on_resource or resource == on_resource:
        return False
    return (
        resource in role
        and on_resource in to_role
        and on_resource not in role
        and resource not in to_role
    )


def _is_ci_file(relative: str) -> bool:
    parts = relative.split("/")
    name = parts[-1]
    in_workflows = parts[-3:-1] == [".github", "workflows"]
    return (
        (in_workflows and name.endswith((".yml", ".yaml")))
        or name == ".gitlab-ci.yml"
        or name.startswith("Dockerfile")
    )


def _id_order(entry: str) -> tuple[str, int]:
    match = re.match(r"([A-Z]+)(\d*)", entry)
    return (match.group(1), int(match.group(2) or 0)) if match else (entry, 0)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Report what the permitio/permit-io 1.0 upgrade guide concerns in a "
        "Terraform configuration. Reads files only; never writes."
    )
    parser.add_argument("path", nargs="?", default=".", help="directory or .tf file to scan")
    parser.add_argument("--json", action="store_true", help="print one JSON document")
    arguments = parser.parse_args(argv)

    scan = Scan(Path(arguments.path))
    scan.run()
    if arguments.json:
        json.dump(scan.as_json(), sys.stdout, indent=2)
        sys.stdout.write("\n")
    else:
        for finding in scan.findings:
            print(f"{finding.path}:{finding.line} {finding.id} {finding.safety} {finding.message}")
        for warning in scan.warnings:
            print(
                f"warning: {warning['path']}:{warning['line']}: {warning['message']}",
                file=sys.stderr,
            )
        counts = scan.as_json()["summary"]["counts"]
        print(
            f"scanned {scan.files} .tf files: {counts[SAFE]} SAFE, {counts[REVIEW]} NEEDS-REVIEW, "
            f"{counts[INFO]} INFO{'' if scan.complete else '; the scan is incomplete'}",
            file=sys.stderr,
        )
    return 0 if scan.complete else 2


if __name__ == "__main__":
    sys.exit(main())
