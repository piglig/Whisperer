package investigator

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

type Template struct {
	ID            string
	Name          string
	Occupation    string
	AttrsJSON     string
	SkillsJSON    string
	HP            int
	MP            int
	SAN           int
	InventoryJSON string
}

var templates = []Template{
	{
		ID:         "journalist",
		Name:       "Lyra Marsh",
		Occupation: "记者",
		AttrsJSON:  `{"STR":50,"CON":60,"SIZ":55,"DEX":60,"APP":55,"INT":75,"POW":60,"EDU":80}`,
		SkillsJSON: `{"Spot Hidden":55,"Library Use":70,"Listen":45,"Psychology":40,"Fast Talk":50,"Photography":45}`,
		HP:         12, MP: 12, SAN: 60,
		InventoryJSON: `["笔记本","钢笔","便携相机","记者证"]`,
	},
	{
		ID:         "private_eye",
		Name:       "Elias Ward",
		Occupation: "私家侦探",
		AttrsJSON:  `{"STR":60,"CON":65,"SIZ":60,"DEX":65,"APP":45,"INT":70,"POW":55,"EDU":65}`,
		SkillsJSON: `{"Spot Hidden":70,"Listen":55,"Psychology":45,"Locksmith":45,"Stealth":50,"Firearms":45}`,
		HP:         12, MP: 11, SAN: 55,
		InventoryJSON: `["手电筒","开锁工具","旧左轮","案件笔记"]`,
	},
	{
		ID:         "doctor",
		Name:       "Mara Bell",
		Occupation: "医生",
		AttrsJSON:  `{"STR":45,"CON":55,"SIZ":50,"DEX":55,"APP":60,"INT":75,"POW":65,"EDU":85}`,
		SkillsJSON: `{"Medicine":75,"First Aid":70,"Psychology":55,"Library Use":55,"Spot Hidden":45,"Science":60}`,
		HP:         11, MP: 13, SAN: 65,
		InventoryJSON: `["医疗包","听诊器","处方笺","钢笔"]`,
	},
}

func Default() Template {
	return templates[0]
}

func ByID(id string) (Template, bool) {
	id = strings.TrimSpace(id)
	for _, tmpl := range templates {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return Template{}, false
}

func NewFromTemplate(saveID string, tmpl Template) store.Investigator {
	return store.Investigator{
		ID:            uuid.NewString(),
		SaveID:        saveID,
		Name:          tmpl.Name,
		Occupation:    tmpl.Occupation,
		AttrsJSON:     tmpl.AttrsJSON,
		SkillsJSON:    tmpl.SkillsJSON,
		HP:            tmpl.HP,
		MP:            tmpl.MP,
		SAN:           tmpl.SAN,
		InventoryJSON: tmpl.InventoryJSON,
		Active:        true,
	}
}

func NewQuick(saveID, name, occupation string) store.Investigator {
	tmpl := Default()
	name = strings.TrimSpace(name)
	if name == "" {
		name = tmpl.Name
	}
	occupation = strings.TrimSpace(occupation)
	if occupation == "" {
		occupation = tmpl.Occupation
	}
	inv := NewFromTemplate(saveID, tmpl)
	inv.Name = name
	inv.Occupation = occupation
	return inv
}

func KnownIDs() []string {
	out := make([]string, len(templates))
	for i, tmpl := range templates {
		out[i] = tmpl.ID
	}
	return out
}

func ErrUnknownTemplate(id string) error {
	return fmt.Errorf("unknown investigator template %q; known templates: %s", id, strings.Join(KnownIDs(), ", "))
}
