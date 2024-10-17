// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package vdom

import (
	"context"
	"fmt"
	"log"
	"reflect"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

type vdomContextKeyType struct{}

var vdomContextKey = vdomContextKeyType{}

type VDomContextVal struct {
	Root    *RootElem
	Comp    *Component
	HookIdx int
}

type Atom struct {
	Val    any
	Dirty  bool
	UsedBy map[string]bool // component waveid -> true
}

type RootElem struct {
	OuterCtx        context.Context
	Root            *Component
	CFuncs          map[string]CFunc
	CompMap         map[string]*Component // component waveid -> component
	EffectWorkQueue []*EffectWorkElem
	NeedsRenderMap  map[string]bool
	Atoms           map[string]*Atom
}

const (
	WorkType_Render = "render"
	WorkType_Effect = "effect"
)

type EffectWorkElem struct {
	Id          string
	EffectIndex int
}

func (r *RootElem) AddRenderWork(id string) {
	if r.NeedsRenderMap == nil {
		r.NeedsRenderMap = make(map[string]bool)
	}
	r.NeedsRenderMap[id] = true
}

func (r *RootElem) AddEffectWork(id string, effectIndex int) {
	r.EffectWorkQueue = append(r.EffectWorkQueue, &EffectWorkElem{Id: id, EffectIndex: effectIndex})
}

func MakeRoot() *RootElem {
	return &RootElem{
		Root:    nil,
		CFuncs:  make(map[string]CFunc),
		CompMap: make(map[string]*Component),
		Atoms:   make(map[string]*Atom),
	}
}

func (r *RootElem) GetAtom(name string) *Atom {
	atom, ok := r.Atoms[name]
	if !ok {
		atom = &Atom{UsedBy: make(map[string]bool)}
		r.Atoms[name] = atom
	}
	return atom
}

func (r *RootElem) GetAtomVal(name string) any {
	atom := r.GetAtom(name)
	return atom.Val
}

func (r *RootElem) GetStateSync(full bool) []VDomStateSync {
	stateSync := make([]VDomStateSync, 0)
	for atomName, atom := range r.Atoms {
		if atom.Dirty || full {
			stateSync = append(stateSync, VDomStateSync{Atom: atomName, Value: atom.Val})
			atom.Dirty = false
		}
	}
	return stateSync
}

func (r *RootElem) SetAtomVal(name string, val any, markDirty bool) {
	atom := r.GetAtom(name)
	if !markDirty {
		atom.Val = val
		return
	}
	// try to avoid setting the value and marking as dirty if it's the "same"
	if utilfn.JsonValEqual(val, atom.Val) {
		return
	}
	atom.Val = val
	atom.Dirty = true
}

func (r *RootElem) SetOuterCtx(ctx context.Context) {
	r.OuterCtx = ctx
}

func (r *RootElem) RegisterComponent(name string, cfunc CFunc) {
	r.CFuncs[name] = cfunc
}

func (r *RootElem) Render(elem *VDomElem) {
	log.Printf("Render %s\n", elem.Tag)
	r.render(elem, &r.Root)
}

func (vdf *VDomFunc) CallFn() {
	if vdf.Fn == nil {
		return
	}
	rval := reflect.ValueOf(vdf.Fn)
	if rval.Kind() != reflect.Func {
		return
	}
	rval.Call(nil)
}

func callVDomFn(fnVal any, data any) {
	if fnVal == nil {
		return
	}
	fn := fnVal
	if vdf, ok := fnVal.(*VDomFunc); ok {
		fn = vdf.Fn
	}
	if fn == nil {
		return
	}
	rval := reflect.ValueOf(fn)
	if rval.Kind() != reflect.Func {
		return
	}
	rtype := rval.Type()
	if rtype.NumIn() == 0 {
		rval.Call(nil)
		return
	}
	if rtype.NumIn() == 1 {
		rval.Call([]reflect.Value{reflect.ValueOf(data)})
		return
	}
}

func (r *RootElem) Event(id string, propName string, data any) {
	comp := r.CompMap[id]
	if comp == nil || comp.Elem == nil {
		return
	}
	fnVal := comp.Elem.Props[propName]
	callVDomFn(fnVal, data)
}

// this will be called by the frontend to say the DOM has been mounted
// it will eventually send any updated "refs" to the backend as well
func (r *RootElem) RunWork() {
	workQueue := r.EffectWorkQueue
	r.EffectWorkQueue = nil
	// first, run effect cleanups
	for _, work := range workQueue {
		comp := r.CompMap[work.Id]
		if comp == nil {
			continue
		}
		hook := comp.Hooks[work.EffectIndex]
		if hook.UnmountFn != nil {
			hook.UnmountFn()
		}
	}
	// now run, new effects
	for _, work := range workQueue {
		comp := r.CompMap[work.Id]
		if comp == nil {
			continue
		}
		hook := comp.Hooks[work.EffectIndex]
		if hook.Fn != nil {
			hook.UnmountFn = hook.Fn()
		}
	}
	// now check if we need a render
	if len(r.NeedsRenderMap) > 0 {
		r.NeedsRenderMap = nil
		r.render(r.Root.Elem, &r.Root)
	}
}

func (r *RootElem) render(elem *VDomElem, comp **Component) {
	if elem == nil || elem.Tag == "" {
		r.unmount(comp)
		return
	}
	elemKey := elem.Key()
	if *comp == nil || !(*comp).compMatch(elem.Tag, elemKey) {
		r.unmount(comp)
		r.createComp(elem.Tag, elemKey, comp)
	}
	(*comp).Elem = elem
	if elem.Tag == TextTag {
		r.renderText(elem.Text, comp)
		return
	}
	if isBaseTag(elem.Tag) {
		// simple vdom, fragment, wave element
		r.renderSimple(elem, comp)
		return
	}
	cfunc := r.CFuncs[elem.Tag]
	if cfunc == nil {
		text := fmt.Sprintf("<%s>", elem.Tag)
		r.renderText(text, comp)
		return
	}
	r.renderComponent(cfunc, elem, comp)
}

func (r *RootElem) unmount(comp **Component) {
	if *comp == nil {
		return
	}
	// parent clean up happens first
	for _, hook := range (*comp).Hooks {
		if hook.UnmountFn != nil {
			hook.UnmountFn()
		}
	}
	// clean up any children
	if (*comp).Comp != nil {
		r.unmount(&(*comp).Comp)
	}
	if (*comp).Children != nil {
		for _, child := range (*comp).Children {
			r.unmount(&child)
		}
	}
	delete(r.CompMap, (*comp).WaveId)
	*comp = nil
}

func (r *RootElem) createComp(tag string, key string, comp **Component) {
	*comp = &Component{WaveId: uuid.New().String(), Tag: tag, Key: key}
	r.CompMap[(*comp).WaveId] = *comp
}

func (r *RootElem) renderText(text string, comp **Component) {
	if (*comp).Text != text {
		(*comp).Text = text
	}
}

func (r *RootElem) renderChildren(elems []VDomElem, curChildren []*Component) []*Component {
	newChildren := make([]*Component, len(elems))
	curCM := make(map[ChildKey]*Component)
	usedMap := make(map[*Component]bool)
	for idx, child := range curChildren {
		if child.Key != "" {
			curCM[ChildKey{Tag: child.Tag, Idx: 0, Key: child.Key}] = child
		} else {
			curCM[ChildKey{Tag: child.Tag, Idx: idx, Key: ""}] = child
		}
	}
	for idx, elem := range elems {
		elemKey := elem.Key()
		var curChild *Component
		if elemKey != "" {
			curChild = curCM[ChildKey{Tag: elem.Tag, Idx: 0, Key: elemKey}]
		} else {
			curChild = curCM[ChildKey{Tag: elem.Tag, Idx: idx, Key: ""}]
		}
		usedMap[curChild] = true
		newChildren[idx] = curChild
		r.render(&elem, &newChildren[idx])
	}
	for _, child := range curChildren {
		if !usedMap[child] {
			r.unmount(&child)
		}
	}
	return newChildren
}

func (r *RootElem) renderSimple(elem *VDomElem, comp **Component) {
	if (*comp).Comp != nil {
		r.unmount(&(*comp).Comp)
	}
	(*comp).Children = r.renderChildren(elem.Children, (*comp).Children)
}

func (r *RootElem) makeRenderContext(comp *Component) context.Context {
	var ctx context.Context
	if r.OuterCtx != nil {
		ctx = r.OuterCtx
	} else {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, vdomContextKey, &VDomContextVal{Root: r, Comp: comp, HookIdx: 0})
	return ctx
}

func getRenderContext(ctx context.Context) *VDomContextVal {
	v := ctx.Value(vdomContextKey)
	if v == nil {
		return nil
	}
	return v.(*VDomContextVal)
}

func (r *RootElem) renderComponent(cfunc CFunc, elem *VDomElem, comp **Component) {
	if (*comp).Children != nil {
		for _, child := range (*comp).Children {
			r.unmount(&child)
		}
		(*comp).Children = nil
	}
	props := make(map[string]any)
	for k, v := range elem.Props {
		props[k] = v
	}
	props[ChildrenPropKey] = elem.Children
	ctx := r.makeRenderContext(*comp)
	renderedElem := cfunc(ctx, props)
	rtnElemArr := partToElems(renderedElem)
	if len(rtnElemArr) == 0 {
		r.unmount(&(*comp).Comp)
		return
	}
	var rtnElem *VDomElem
	if len(rtnElemArr) == 1 {
		rtnElem = &rtnElemArr[0]
	} else {
		rtnElem = &VDomElem{Tag: FragmentTag, Children: rtnElemArr}
	}
	r.render(rtnElem, &(*comp).Comp)
}

func convertPropsToVDom(props map[string]any) map[string]any {
	if len(props) == 0 {
		return nil
	}
	vdomProps := make(map[string]any)
	for k, v := range props {
		if v == nil {
			continue
		}
		val := reflect.ValueOf(v)
		if val.Kind() == reflect.Func {
			vdomProps[k] = VDomFunc{Type: ObjectType_Func}
			continue
		}
		vdomProps[k] = v
	}
	return vdomProps
}

func convertBaseToVDom(c *Component) *VDomElem {
	elem := &VDomElem{WaveId: c.WaveId, Tag: c.Tag}
	if c.Elem != nil {
		elem.Props = convertPropsToVDom(c.Elem.Props)
	}
	for _, child := range c.Children {
		childVDom := convertToVDom(child)
		if childVDom != nil {
			elem.Children = append(elem.Children, *childVDom)
		}
	}
	return elem
}

func convertToVDom(c *Component) *VDomElem {
	if c == nil {
		return nil
	}
	if c.Tag == TextTag {
		return &VDomElem{Tag: TextTag, Text: c.Text}
	}
	if isBaseTag(c.Tag) {
		return convertBaseToVDom(c)
	} else {
		return convertToVDom(c.Comp)
	}
}

func (r *RootElem) makeVDom(comp *Component) *VDomElem {
	vdomElem := convertToVDom(comp)
	return vdomElem
}

func (r *RootElem) MakeVDom() *VDomElem {
	return r.makeVDom(r.Root)
}
