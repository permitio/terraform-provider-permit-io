package providerschema

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The plan modifier entries a snapshot records for the framework's
// stringplanmodifier.RequiresReplace and UseStateForUnknown.
const (
	requiresReplace = "stringplanmodifier.requiresReplaceIfModifier: If the value of this " +
		"attribute changes, Terraform will destroy and recreate the resource."
	useStateForUnknown = "stringplanmodifier.useStateForUnknownModifier: Once set, the value " +
		"of this attribute in state will not change."
)

// baseFixture is the base snapshot every diff case starts from.
func baseFixture() Snapshot {
	return Snapshot{
		Provider: Schema{Attributes: map[string]Attribute{
			"api_key": {Type: "string", Optional: true, Sensitive: true},
		}},
		Resources: map[string]Schema{
			"example_thing": {
				Description: "A thing.",
				Attributes: map[string]Attribute{
					"id": {Type: "string", Computed: true,
						PlanModifiers: []string{useStateForUnknown}},
					"key": {Type: "string", Required: true,
						PlanModifiers: []string{requiresReplace}},
					"name":      {Type: "string", Optional: true, Description: "The name."},
					"tags":      {Type: "set(string)", Optional: true, Computed: true},
					"level":     {Type: "number", Optional: true, Default: "value defaults to 1"},
					"rules":     {Type: "list_nested", Optional: true},
					"rules.url": {Type: "string", Required: true},
					"steps.run": {Type: "string", Optional: true},
				},
				Blocks: map[string]Block{
					"steps": {Nesting: "list", MaxItems: 5},
					"hooks": {Nesting: "set", MinItems: 1},
				},
			},
			"example_other": {Attributes: map[string]Attribute{
				"id": {Type: "string", Computed: true},
			}},
		},
		DataSources: map[string]Schema{
			"example_thing": {Attributes: map[string]Attribute{
				"key": {Type: "string", Required: true},
			}},
		},
	}
}

// clone deep-copies a snapshot through its JSON form.
func clone(t *testing.T, snapshot Snapshot) Snapshot {
	t.Helper()
	data, err := Encode(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return copied
}

func TestDiff(t *testing.T) {
	thing := func(s *Snapshot) Schema { return s.Resources["example_thing"] }
	setAttribute := func(s *Snapshot, path string, change func(*Attribute)) {
		a := thing(s).Attributes[path]
		change(&a)
		thing(s).Attributes[path] = a
	}
	setBlock := func(s *Snapshot, path string, change func(*Block)) {
		b := thing(s).Blocks[path]
		change(&b)
		thing(s).Blocks[path] = b
	}
	const subject = "resource example_thing: "

	tests := []struct {
		name   string
		change func(*Snapshot)
		want   []string
	}{
		{
			name:   "no change",
			change: func(*Snapshot) {},
		},
		{
			name:   "attribute removed",
			change: func(s *Snapshot) { delete(thing(s).Attributes, "name") },
			want:   []string{"BREAKING      " + subject + `attribute "name" removed`},
		},
		{
			name: "optional to required",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) { a.Optional, a.Required = false, true })
			},
			want: []string{"BREAKING      " + subject + `attribute "name" changed from ` +
				"optional to required: configurations that leave it unset now fail"},
		},
		{
			name: "RequiresReplace added",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) {
					a.PlanModifiers = []string{requiresReplace}
				})
			},
			want: []string{"BREAKING      " + subject + `attribute "name" now forces ` +
				"replacement: " + requiresReplace},
		},
		{
			name: "RequiresReplaceIf added with a description that does not say so",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) {
					a.PlanModifiers = []string{"stringplanmodifier.requiresReplaceIfModifier: " +
						"Changing it makes a new object."}
				})
			},
			want: []string{"BREAKING      " + subject + `attribute "name" now forces ` +
				"replacement: stringplanmodifier.requiresReplaceIfModifier: Changing it makes " +
				"a new object."},
		},
		{
			name: "custom plan modifier that says it replaces",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) {
					a.PlanModifiers = []string{"example.regionModifier: Terraform will " +
						"replace the resource when the region changes."}
				})
			},
			want: []string{"BREAKING      " + subject + `attribute "name" now forces ` +
				"replacement: example.regionModifier: Terraform will replace the resource " +
				"when the region changes."},
		},
		{
			name: "custom plan modifier that says it recreates",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) {
					a.PlanModifiers = []string{"example.rebuildModifier: Terraform will " +
						"destroy and recreate the resource."}
				})
			},
			want: []string{"BREAKING      " + subject + `attribute "name" now forces ` +
				"replacement: example.rebuildModifier: Terraform will destroy and recreate " +
				"the resource."},
		},
		{
			name: "custom plan modifier that does not force replacement",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) {
					a.PlanModifiers = []string{"example.keepModifier: Keeps the prior value."}
				})
			},
			want: []string{"non-breaking  " + subject + `attribute "name" plan modifier ` +
				"added: example.keepModifier: Keeps the prior value."},
		},
		{
			name: "block RequiresReplace added",
			change: func(s *Snapshot) {
				setBlock(s, "steps", func(b *Block) { b.PlanModifiers = []string{requiresReplace} })
			},
			want: []string{"BREAKING      " + subject + `block "steps" now forces ` +
				"replacement: " + requiresReplace},
		},
		{
			name: "optional attribute added",
			change: func(s *Snapshot) {
				thing(s).Attributes["color"] = Attribute{Type: "string", Optional: true}
			},
			want: []string{"non-breaking  " + subject + `attribute "color" added (optional)`},
		},
		{
			name: "computed attribute added",
			change: func(s *Snapshot) {
				thing(s).Attributes["created_at"] = Attribute{Type: "string", Computed: true}
			},
			want: []string{"non-breaking  " + subject + `attribute "created_at" added (computed)`},
		},
		{
			name: "required attribute added",
			change: func(s *Snapshot) {
				thing(s).Attributes["owner"] = Attribute{Type: "string", Required: true}
			},
			want: []string{"BREAKING      " + subject + `new required attribute "owner"`},
		},
		{
			name: "required attribute added inside an existing nested attribute",
			change: func(s *Snapshot) {
				thing(s).Attributes["rules.method"] = Attribute{Type: "string", Required: true}
			},
			want: []string{"BREAKING      " + subject + `new required attribute "rules.method"`},
		},
		{
			name: "nested attribute removed with its children",
			change: func(s *Snapshot) {
				delete(thing(s).Attributes, "rules")
				delete(thing(s).Attributes, "rules.url")
			},
			want: []string{"BREAKING      " + subject + `attribute "rules" removed`},
		},
		{
			name: "optional nested attribute added with a required child",
			change: func(s *Snapshot) {
				thing(s).Attributes["auth"] = Attribute{Type: "single_nested", Optional: true}
				thing(s).Attributes["auth.token"] = Attribute{Type: "string", Required: true}
			},
			want: []string{"non-breaking  " + subject + `attribute "auth" added (optional)`},
		},
		{
			name: "type changed",
			change: func(s *Snapshot) {
				setAttribute(s, "tags", func(a *Attribute) { a.Type = "list(string)" })
			},
			want: []string{"BREAKING      " + subject + `attribute "tags" type changed from ` +
				"set(string) to list(string)"},
		},
		{
			name: "computed lost",
			change: func(s *Snapshot) {
				setAttribute(s, "tags", func(a *Attribute) { a.Computed = false })
			},
			want: []string{"BREAKING      " + subject + `attribute "tags" changed from ` +
				"optional+computed to optional: it is no longer computed, so configurations " +
				"that leave it unset get null"},
		},
		{
			name: "no longer settable",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) { a.Optional, a.Computed = false, true })
			},
			want: []string{"BREAKING      " + subject + `attribute "name" changed from ` +
				"optional to computed: configurations that set it now fail"},
		},
		{
			name: "required made computed",
			change: func(s *Snapshot) {
				setAttribute(s, "key", func(a *Attribute) { a.Required, a.Computed = false, true })
			},
			want: []string{"BREAKING      " + subject + `attribute "key" changed from ` +
				"required to computed: configurations that set it now fail"},
		},
		{
			name: "computed made settable",
			change: func(s *Snapshot) {
				setAttribute(s, "id", func(a *Attribute) { a.Optional = true })
			},
			want: []string{"non-breaking  " + subject + `attribute "id" changed from ` +
				"computed to optional+computed"},
		},
		{
			name: "required made optional",
			change: func(s *Snapshot) {
				setAttribute(s, "key", func(a *Attribute) { a.Required, a.Optional = false, true })
			},
			want: []string{"non-breaking  " + subject + `attribute "key" changed from ` +
				"required to optional"},
		},
		{
			name: "sensitive flipped",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) { a.Sensitive = true })
			},
			want: []string{"BREAKING      " + subject + `attribute "name" sensitive changed ` +
				"from false to true"},
		},
		{
			name: "write-only flipped",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) { a.WriteOnly = true })
			},
			want: []string{"BREAKING      " + subject + `attribute "name" write-only changed ` +
				"from false to true"},
		},
		{
			name: "attribute deprecated",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) { a.DeprecationMessage = "Use key." })
			},
			want: []string{"non-breaking  " + subject + `attribute "name" deprecated: Use key.`},
		},
		{
			name: "description and default changed",
			change: func(s *Snapshot) {
				setAttribute(s, "name", func(a *Attribute) { a.Description = "The display name." })
				setAttribute(s, "level", func(a *Attribute) { a.Default = "" })
			},
			want: []string{
				"non-breaking  " + subject + `attribute "level" default changed from ` +
					`"value defaults to 1" to none`,
				"non-breaking  " + subject + `attribute "name" description changed`,
			},
		},
		{
			name: "RequiresReplace removed, other plan modifier and validator added",
			change: func(s *Snapshot) {
				setAttribute(s, "key", func(a *Attribute) {
					a.PlanModifiers = []string{useStateForUnknown}
					a.Validators = []string{"string length must be at least 1"}
				})
			},
			want: []string{
				"non-breaking  " + subject + `attribute "key" plan modifier added: ` +
					useStateForUnknown,
				"non-breaking  " + subject + `attribute "key" plan modifier removed: ` +
					requiresReplace,
				"non-breaking  " + subject + `attribute "key" validator added: string ` +
					"length must be at least 1",
			},
		},
		{
			name: "block removed with its attributes",
			change: func(s *Snapshot) {
				delete(thing(s).Blocks, "steps")
				delete(thing(s).Attributes, "steps.run")
			},
			want: []string{"BREAKING      " + subject + `block "steps" removed`},
		},
		{
			name: "blocks added",
			change: func(s *Snapshot) {
				thing(s).Blocks["timeouts"] = Block{Nesting: "single"}
				thing(s).Blocks["target"] = Block{Nesting: "list", MinItems: 1}
				thing(s).Attributes["target.url"] = Attribute{Type: "string", Required: true}
			},
			want: []string{
				"BREAKING      " + subject + `new required block "target" (min_items 1)`,
				"non-breaking  " + subject + `block "timeouts" added`,
			},
		},
		{
			name: "block nesting changed and max_items tightened",
			change: func(s *Snapshot) {
				setBlock(s, "steps", func(b *Block) { b.Nesting, b.MaxItems = "set", 2 })
			},
			want: []string{
				"BREAKING      " + subject + `block "steps" nesting changed from list to set`,
				"BREAKING      " + subject + `block "steps" max_items changed from 5 to 2`,
			},
		},
		{
			name: "block limits relaxed",
			change: func(s *Snapshot) {
				setBlock(s, "steps", func(b *Block) { b.MaxItems = 0 })
			},
			want: []string{"non-breaking  " + subject + `block "steps" max_items changed ` +
				"from 5 to unlimited"},
		},
		{
			name: "block min_items raised",
			change: func(s *Snapshot) {
				setBlock(s, "steps", func(b *Block) { b.MinItems = 1 })
			},
			want: []string{"BREAKING      " + subject + `block "steps" min_items changed ` +
				"from 0 to 1"},
		},
		{
			name: "block min_items lowered",
			change: func(s *Snapshot) {
				setBlock(s, "hooks", func(b *Block) { b.MinItems = 0 })
			},
			want: []string{"non-breaking  " + subject + `block "hooks" min_items changed ` +
				"from 1 to 0"},
		},
		{
			name: "block max_items limited",
			change: func(s *Snapshot) {
				setBlock(s, "hooks", func(b *Block) { b.MaxItems = 3 })
			},
			want: []string{"BREAKING      " + subject + `block "hooks" max_items changed ` +
				"from unlimited to 3"},
		},
		{
			name: "resource removed, data source added, provider attribute added",
			change: func(s *Snapshot) {
				delete(s.Resources, "example_other")
				s.DataSources["example_other"] = Schema{Attributes: map[string]Attribute{
					"id": {Type: "string", Required: true},
				}}
				s.Provider.Attributes["timeout"] = Attribute{Type: "number", Optional: true}
			},
			want: []string{
				`non-breaking  provider: attribute "timeout" added (optional)`,
				"BREAKING      resource example_other: removed",
				"non-breaking  data source example_other: added",
			},
		},
		{
			name: "resource deprecated and versioned",
			change: func(s *Snapshot) {
				other := s.Resources["example_other"]
				other.DeprecationMessage = "Use example_thing."
				other.Version = 1
				s.Resources["example_other"] = other
			},
			want: []string{
				"non-breaking  resource example_other: schema version changed from 0 to 1",
				"non-breaking  resource example_other: deprecated: Use example_thing.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := baseFixture()
			head := clone(t, base)
			tt.change(&head)
			var got []string
			for _, change := range Diff(base, head) {
				got = append(got, change.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Diff() =\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"),
					strings.Join(tt.want, "\n\t"))
			}
		})
	}
}

// TestDiffReportsEveryField changes each field of an attribute, a block and a
// schema in turn and checks that Diff reports it, so that a field added to the
// snapshot format cannot change without showing up in the diff.
func TestDiffReportsEveryField(t *testing.T) {
	base := baseFixture()
	checked := 0
	name := base.Resources["example_thing"].Attributes["name"]
	for field, attribute := range oneFieldChanged(t, name) {
		head := clone(t, base)
		head.Resources["example_thing"].Attributes["name"] = attribute
		if len(Diff(base, head)) == 0 {
			t.Errorf("a change to Attribute.%s is not reported", field)
		}
		checked++
	}
	for field, block := range oneFieldChanged(t, base.Resources["example_thing"].Blocks["steps"]) {
		head := clone(t, base)
		head.Resources["example_thing"].Blocks["steps"] = block
		if len(Diff(base, head)) == 0 {
			t.Errorf("a change to Block.%s is not reported", field)
		}
		checked++
	}
	for field, schema := range oneFieldChanged(t, base.Resources["example_other"]) {
		head := clone(t, base)
		head.Resources["example_other"] = schema
		if len(Diff(base, head)) == 0 {
			t.Errorf("a change to Schema.%s is not reported", field)
		}
		checked++
	}
	if want := 11 + 7 + 3; checked != want {
		t.Fatalf("checked %d fields, want %d; update the count when the format changes",
			checked, want)
	}
}

// oneFieldChanged returns copies of value keyed by field name, each with that one
// field changed. Map fields, which hold nested attributes and blocks, are left to
// the add and remove cases of TestDiff.
func oneFieldChanged[T any](t *testing.T, value T) map[string]T {
	t.Helper()
	variants := map[string]T{}
	fields := reflect.TypeFor[T]()
	for i := range fields.NumField() {
		changed := value
		field := reflect.ValueOf(&changed).Elem().Field(i)
		switch field.Kind() {
		case reflect.Map:
			continue
		case reflect.String:
			field.SetString(field.String() + " changed")
		case reflect.Bool:
			field.SetBool(!field.Bool())
		case reflect.Int64:
			field.SetInt(field.Int() + 1)
		case reflect.Slice:
			field.Set(reflect.Append(field, reflect.ValueOf("changed")))
		default:
			t.Fatalf("%s.%s has kind %s, which this test cannot change", fields.Name(),
				fields.Field(i).Name, field.Kind())
		}
		variants[fields.Field(i).Name] = changed
	}
	return variants
}
