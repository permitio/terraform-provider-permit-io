package providerschema

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Change is one difference between a base and a head snapshot.
type Change struct {
	// Breaking is true when a configuration or state written for the base can
	// fail, lose a value or be replaced under the head.
	Breaking bool
	// Subject is "provider", "resource <name>" or "data source <name>".
	Subject string
	Detail  string
}

func (c Change) String() string {
	label := "non-breaking"
	if c.Breaking {
		label = "BREAKING"
	}
	return fmt.Sprintf("%-12s  %s: %s", label, c.Subject, c.Detail)
}

// Diff lists every difference from base to head, grouped by the provider, the
// resources and the data sources, each in name order. Within one schema it lists
// the schema's own changes, then its attributes' and then its blocks', each in
// path order.
//
// A change is breaking when it removes a resource, data source, attribute or
// block; adds a required attribute or a block with min_items; changes a type or
// block nesting; makes an attribute required, no longer settable or no longer
// computed; flips sensitive or write-only; tightens min_items or max_items; or
// adds a plan modifier that forces replacement. Any other difference, such as an
// added optional attribute, a deprecation, or a changed description, validator or
// default, is listed as non-breaking. Attributes and blocks under a removed or
// added parent are covered by the parent's change and not listed again.
func Diff(base, head Snapshot) []Change {
	d := differ{}
	d.subject = "provider"
	d.schema(base.Provider, head.Provider)
	d.schemas("resource", base.Resources, head.Resources)
	d.schemas("data source", base.DataSources, head.DataSources)
	return d.changes
}

type differ struct {
	subject string
	changes []Change
}

func (d *differ) add(breaking bool, format string, args ...any) {
	d.changes = append(d.changes, Change{
		Breaking: breaking,
		Subject:  d.subject,
		Detail:   fmt.Sprintf(format, args...),
	})
}

func (d *differ) schemas(kind string, base, head map[string]Schema) {
	for _, name := range unionKeys(base, head) {
		d.subject = kind + " " + name
		baseSchema, inBase := base[name]
		headSchema, inHead := head[name]
		switch {
		case !inHead:
			d.add(true, "removed")
		case !inBase:
			d.add(false, "added")
		default:
			d.schema(baseSchema, headSchema)
		}
	}
}

func (d *differ) schema(base, head Schema) {
	if base.Version != head.Version {
		d.add(false, "schema version changed from %d to %d", base.Version, head.Version)
	}
	if base.Description != head.Description {
		d.add(false, "description changed")
	}
	d.deprecation("", base.DeprecationMessage, head.DeprecationMessage)

	inBase := func(path string) bool { return has(base, path) }
	inHead := func(path string) bool { return has(head, path) }
	for _, path := range unionKeys(base.Attributes, head.Attributes) {
		baseAttribute, wasThere := base.Attributes[path]
		headAttribute, isThere := head.Attributes[path]
		what := fmt.Sprintf("attribute %q", path)
		switch {
		case !isThere:
			if parentOnlyIn(path, inBase, inHead) {
				continue
			}
			d.add(true, "%s removed", what)
		case !wasThere:
			if parentOnlyIn(path, inHead, inBase) {
				continue
			}
			if headAttribute.Required {
				d.add(true, "new required %s", what)
			} else {
				d.add(false, "%s added (%s)", what, mode(headAttribute))
			}
		default:
			d.attribute(what, baseAttribute, headAttribute)
		}
	}
	for _, path := range unionKeys(base.Blocks, head.Blocks) {
		baseBlock, wasThere := base.Blocks[path]
		headBlock, isThere := head.Blocks[path]
		what := fmt.Sprintf("block %q", path)
		switch {
		case !isThere:
			if parentOnlyIn(path, inBase, inHead) {
				continue
			}
			d.add(true, "%s removed", what)
		case !wasThere:
			if parentOnlyIn(path, inHead, inBase) {
				continue
			}
			if headBlock.MinItems > 0 {
				d.add(true, "new required %s (min_items %d)", what, headBlock.MinItems)
			} else {
				d.add(false, "%s added", what)
			}
		default:
			d.block(what, baseBlock, headBlock)
		}
	}
}

func (d *differ) attribute(what string, base, head Attribute) {
	if base.Type != head.Type {
		d.add(true, "%s type changed from %s to %s", what, base.Type, head.Type)
	}
	if mode(base) != mode(head) {
		if reason := modeBreak(base, head); reason != "" {
			d.add(true, "%s changed from %s to %s: %s", what, mode(base), mode(head), reason)
		} else {
			d.add(false, "%s changed from %s to %s", what, mode(base), mode(head))
		}
	}
	if base.Sensitive != head.Sensitive {
		d.add(true, "%s sensitive changed from %t to %t", what, base.Sensitive, head.Sensitive)
	}
	if base.WriteOnly != head.WriteOnly {
		d.add(true, "%s write-only changed from %t to %t", what, base.WriteOnly, head.WriteOnly)
	}
	d.deprecation(what, base.DeprecationMessage, head.DeprecationMessage)
	if base.Description != head.Description {
		d.add(false, "%s description changed", what)
	}
	if base.Default != head.Default {
		d.add(false, "%s default changed from %s to %s", what, orNone(base.Default),
			orNone(head.Default))
	}
	d.planModifiers(what, base.PlanModifiers, head.PlanModifiers)
	d.validators(what, base.Validators, head.Validators)
}

func (d *differ) block(what string, base, head Block) {
	if base.Nesting != head.Nesting {
		d.add(true, "%s nesting changed from %s to %s", what, base.Nesting, head.Nesting)
	}
	if base.MinItems != head.MinItems {
		d.add(head.MinItems > base.MinItems, "%s min_items changed from %d to %d", what,
			base.MinItems, head.MinItems)
	}
	if base.MaxItems != head.MaxItems {
		tightened := head.MaxItems != 0 && (base.MaxItems == 0 || head.MaxItems < base.MaxItems)
		d.add(tightened, "%s max_items changed from %s to %s", what, maxItems(base.MaxItems),
			maxItems(head.MaxItems))
	}
	d.deprecation(what, base.DeprecationMessage, head.DeprecationMessage)
	if base.Description != head.Description {
		d.add(false, "%s description changed", what)
	}
	d.planModifiers(what, base.PlanModifiers, head.PlanModifiers)
	d.validators(what, base.Validators, head.Validators)
}

// deprecation reports a deprecation added, removed or reworded. None of these
// breaks a configuration; Terraform only warns.
func (d *differ) deprecation(what, base, head string) {
	prefix := ""
	if what != "" {
		prefix = what + " "
	}
	switch {
	case base == head:
	case base == "":
		d.add(false, "%sdeprecated: %s", prefix, head)
	case head == "":
		d.add(false, "%sno longer deprecated", prefix)
	default:
		d.add(false, "%sdeprecation message changed to: %s", prefix, head)
	}
}

func (d *differ) planModifiers(what string, base, head []string) {
	added, removed := listDiff(base, head)
	for _, modifier := range added {
		if forcesReplacement(modifier) {
			d.add(true, "%s now forces replacement: %s", what, modifier)
		} else {
			d.add(false, "%s plan modifier added: %s", what, modifier)
		}
	}
	for _, modifier := range removed {
		d.add(false, "%s plan modifier removed: %s", what, modifier)
	}
}

func (d *differ) validators(what string, base, head []string) {
	added, removed := listDiff(base, head)
	for _, validator := range added {
		d.add(false, "%s validator added: %s", what, validator)
	}
	for _, validator := range removed {
		d.add(false, "%s validator removed: %s", what, validator)
	}
}

// forcesReplacement recognises a plan modifier, recorded as its Go type and
// description, that can force replacement. The framework's RequiresReplace,
// RequiresReplaceIf and RequiresReplaceIfConfigured all have the type
// requiresReplaceIfModifier, whatever description they were given. A custom
// modifier is recognised when its type or description mentions replacing or
// recreating. A modifier that mentions replacing something else is labelled
// breaking too, which a reviewer can dismiss; a missed one would go unreviewed.
func forcesReplacement(modifier string) bool {
	lower := strings.ToLower(modifier)
	return strings.Contains(lower, "replace") || strings.Contains(lower, "recreate")
}

// mode says how a configuration can use an attribute.
func mode(a Attribute) string {
	switch {
	case a.Required:
		return "required"
	case a.Optional && a.Computed:
		return "optional+computed"
	case a.Optional:
		return "optional"
	case a.Computed:
		return "computed"
	}
	return "unset"
}

// modeBreak returns why a change of mode breaks a configuration, or "" when it
// does not.
func modeBreak(base, head Attribute) string {
	settable := func(a Attribute) bool { return a.Required || a.Optional }
	switch {
	case head.Required && !base.Required:
		return "configurations that leave it unset now fail"
	case settable(base) && !settable(head):
		return "configurations that set it now fail"
	case base.Computed && !head.Computed:
		return "it is no longer computed, so configurations that leave it unset get null"
	}
	return ""
}

// has reports whether path is an attribute or a block of schema.
func has(schema Schema, path string) bool {
	_, isAttribute := schema.Attributes[path]
	_, isBlock := schema.Blocks[path]
	return isAttribute || isBlock
}

// parentOnlyIn reports whether an attribute or block that path is nested in is in
// one snapshot and not the other, so its own change already covers path.
func parentOnlyIn(path string, in, notIn func(string) bool) bool {
	for i := strings.LastIndex(path, "."); i > 0; i = strings.LastIndex(path[:i], ".") {
		parent := path[:i]
		if in(parent) && !notIn(parent) {
			return true
		}
	}
	return false
}

// listDiff returns the entries of head missing from base and of base missing from
// head, counting repeated entries.
func listDiff(base, head []string) (added, removed []string) {
	counts := map[string]int{}
	for _, entry := range base {
		counts[entry]++
	}
	for _, entry := range head {
		counts[entry]--
	}
	for _, entry := range slices.Sorted(maps.Keys(counts)) {
		for n := counts[entry]; n < 0; n++ {
			added = append(added, entry)
		}
		for n := counts[entry]; n > 0; n-- {
			removed = append(removed, entry)
		}
	}
	return added, removed
}

func unionKeys[V any](a, b map[string]V) []string {
	keys := slices.Collect(maps.Keys(a))
	for key := range b {
		if _, ok := a[key]; !ok {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return fmt.Sprintf("%q", value)
}

func maxItems(n int64) string {
	if n == 0 {
		return "unlimited"
	}
	return fmt.Sprint(n)
}
