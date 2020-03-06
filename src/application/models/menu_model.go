package models

import (
	"github.com/xiusin/pine/di"
	"log"
	"strconv"
	"strings"

	"github.com/go-xorm/xorm"
	tables "github.com/xiusin/pinecms/src/application/models/tables"
)

type MenuModel struct {
	orm *xorm.Engine
}

func NewMenuModel() *MenuModel {
	return &MenuModel{orm: di.MustGet("*xorm.Engine").(*xorm.Engine)}
}

//根据父级ID获取菜单列表 不递归
func (m *MenuModel) GetMenu(parentid, roleid int64) []tables.IriscmsMenu {
	menus := new([]tables.IriscmsMenu)
	m.orm.Where("parentid = ? and display= ?", parentid, 1).Asc("listorder").Find(menus)
	if roleid == 1 {
		return *menus
	}
	retmenus := []tables.IriscmsMenu{}
	//结合角色权限进行菜单返回
	for _, menu := range *menus {
		total, _ := m.orm.Where("c=? and a=? and roleid=?", menu.C, menu.A, roleid).Count(&tables.IriscmsAdminRolePriv{})
		//public的操作也要全部暴露
		if total > 0 || strings.Contains(menu.A, "public-") {
			retmenus = append(retmenus, menu)
		}
	}
	return retmenus
}

//当前位置
func (m MenuModel) CurrentPos(id int64) string {
	menu := tables.IriscmsMenu{Id: id}
	has, _ := m.orm.Get(&menu)
	str := ""
	if !has {
		return ""
	}
	if menu.Parentid != 0 {
		str = m.CurrentPos(menu.Parentid)
	}
	return str + menu.Name + " > "
}

func (m MenuModel) GetTree(menus []tables.IriscmsMenu, parentid int64) []map[string]interface{} {
	res := []map[string]interface{}{}
	if len(menus) != 0 {
		for _, menu := range menus {
			if parentid == menu.Parentid {
				son := map[string]interface{}{
					"id":        menu.Id,
					"name":      menu.Name,
					"listorder": menu.Listorder,
					"operateid": menu.Id,
					"is_system": menu.IsSystem,
				}
				son["children"] = m.GetTree(menus, menu.Id)
				res = append(res, son)
			}
		}
	}
	return res
}

func (m MenuModel) GetAll() []tables.IriscmsMenu {
	menus := new([]tables.IriscmsMenu)
	m.orm.Asc("listorder").Desc("id").Find(menus)
	return *menus
}

func (m MenuModel) GetRoleTree(parentid int64, roleid int64) []map[string]interface{} {
	menus := new([]tables.IriscmsMenu)
	//过滤我的面板
	err := m.orm.Where("`parentid`=? AND `id`<>?", parentid, 1).Asc("listorder").Desc("id").Find(menus)
	if err != nil {
		log.Println(err.Error())
		return nil
	}
	res := []map[string]interface{}{}
	if len(*menus) != 0 {
		for _, v := range *menus {
			menu := map[string]interface{}{
				"id":   v.Id,
				"text": v.Name,
				"attributes": map[string]interface{}{
					"parent": m.GetParentIds(v.Id, ""),
				},
				"children": m.GetRoleTree(v.Id, roleid),
			}
			if len(menu["children"].([]map[string]interface{})) > 0 {
				menu["state"] = "closed"
			} else {
				//勾选默认菜单
				rolePriv := new([]tables.IriscmsAdminRolePriv)
				m.orm.Where("c=? and a=? and roleid=?", v.C, v.A, roleid).Find(rolePriv)
				if len(*rolePriv) > 0 {
					menu["checked"] = true
				}
			}
			res = append(res, menu)
		}
	}
	return res
}

//获取菜单父级id
func (m MenuModel) GetParentIds(id int64, result string) string {
	menu := tables.IriscmsMenu{Id: id}
	has, _ := m.orm.Get(&menu)
	var parentid int64 = 0
	if has {
		parentid = menu.Parentid
	}
	res := ""
	if parentid != 0 {
		if result == "" {
			res = strconv.Itoa(int(parentid))
		} else {
			res = "," + result
		}
		res = m.GetParentIds(parentid, res)
	}
	return res
}

//检查菜单名称是否存在
func (m MenuModel) CheckName(name string) bool {
	has, _ := m.orm.Get(&tables.IriscmsMenu{Name: name})
	return has
}

//
func (m MenuModel) GetInfo(id int64) (*tables.IriscmsMenu, bool) {
	im := &tables.IriscmsMenu{Id: id}
	has, _ := m.orm.Get(im)
	return im, has
}

//获取selectTree
func (m MenuModel) GetSelectTree(menus []tables.IriscmsMenu, parentid int64) []map[string]interface{} {
	res := []map[string]interface{}{}
	if len(menus) != 0 {
		for _, v := range menus {
			if parentid == v.Parentid {
				menu := map[string]interface{}{
					"id":       v.Id,
					"text":     v.Name,
					"children": m.GetSelectTree(menus, v.Id),
				}
				res = append(res, menu)
			}
		}
	}
	return res
}
