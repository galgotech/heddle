package vm

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"heddle/pkg/lang/ir"
	"heddle/pkg/runtime"
)

type patternMatcher struct {
	vm *VM
}

func (m *patternMatcher) Match(ctx context.Context, targetVal any, cases []ir.MatchCase, scope *runtime.Scope) (*runtime.Frame, bool, error) {
	if f, ok := targetVal.(*runtime.Frame); ok {
		if len(f.Rows) == 1 {
			targetVal = f.Rows[0]
		}
	}

	vVal := reflect.ValueOf(targetVal)
	for vVal.Kind() == reflect.Ptr || vVal.Kind() == reflect.Interface {
		if vVal.IsNil() {
			break
		}
		vVal = vVal.Elem()
	}

	if nextFunc, ok := isSeq(vVal); ok {
		m.vm.incrementActiveEvents()

		if nextFunc.Type().NumIn() == 1 {
			yieldType := nextFunc.Type().In(0)
			var flowResult *runtime.Frame
			var executionErr error

			yieldImpl := reflect.MakeFunc(yieldType, func(args []reflect.Value) []reflect.Value {
				payload := args[0].Interface()
				tag, realPayload := m.extractTagAndPayload(payload)

				var matchedBranch *ir.MatchCase
				if len(cases) == 1 {
					matchedBranch = &cases[0]
				} else {
					for idx := range cases {
						if strings.EqualFold(cases[idx].Tag, tag) || strings.EqualFold(cases[idx].Tag, "each") {
							matchedBranch = &cases[idx]
							break
						}
					}
				}

				if matchedBranch != nil {
					caseScope := runtime.NewScope(scope)
					if matchedBranch.ParamName != "" {
						caseScope.Set(matchedBranch.ParamName, runtime.NewScalarFrame(realPayload))
					}
					resFrame, err := m.vm.executor.ExecuteInstructions(ctx, matchedBranch.Instructions, caseScope)
					if err != nil {
						executionErr = err
						return []reflect.Value{reflect.ValueOf(false)}
					}
					if resFrame != nil {
						flowResult = resFrame
						return []reflect.Value{reflect.ValueOf(false)}
					}
				}
				return []reflect.Value{reflect.ValueOf(true)}
			})

			nextFunc.Call([]reflect.Value{yieldImpl})
			m.vm.decrementActiveEvents()
			if executionErr != nil {
				return nil, false, executionErr
			}
			if flowResult != nil {
				return flowResult, true, nil
			}
			return runtime.NewFrame([]any{}), false, nil
		}

		for {
			select {
			case <-ctx.Done():
				break
			default:
			}

			results := nextFunc.Call(nil)
			if len(results) != 2 {
				break
			}
			okVal := results[1].Bool()
			if !okVal {
				break
			}

			payload := results[0].Interface()
			tag, realPayload := m.extractTagAndPayload(payload)

			var matchedBranch *ir.MatchCase
			if len(cases) == 1 {
				matchedBranch = &cases[0]
			} else {
				for idx := range cases {
					if strings.EqualFold(cases[idx].Tag, tag) || strings.EqualFold(cases[idx].Tag, "each") {
						matchedBranch = &cases[idx]
						break
					}
				}
			}

			if matchedBranch != nil {
				caseScope := runtime.NewScope(scope)
				if matchedBranch.ParamName != "" {
					caseScope.Set(matchedBranch.ParamName, runtime.NewScalarFrame(realPayload))
				}
				resFrame, err := m.vm.executor.ExecuteInstructions(ctx, matchedBranch.Instructions, caseScope)
				if err != nil {
					m.vm.decrementActiveEvents()
					return nil, false, err
				}
				if resFrame != nil {
					m.vm.decrementActiveEvents()
					return resFrame, true, nil
				}
			}
		}

		m.vm.decrementActiveEvents()
		return runtime.NewFrame([]any{}), false, nil
	}

	if nextFunc, ok := isSeqAccum(vVal); ok {
		m.vm.incrementActiveEvents()

		if nextFunc.Type().NumIn() == 1 {
			yieldType := nextFunc.Type().In(0)
			var accumulatedRows []any
			var executionErr error

			yieldImpl := reflect.MakeFunc(yieldType, func(args []reflect.Value) []reflect.Value {
				payload := args[0].Interface()
				tag, realPayload := m.extractTagAndPayload(payload)

				var matchedBranch *ir.MatchCase
				if len(cases) == 1 {
					matchedBranch = &cases[0]
				} else {
					for idx := range cases {
						if strings.EqualFold(cases[idx].Tag, tag) || strings.EqualFold(cases[idx].Tag, "each") {
							matchedBranch = &cases[idx]
							break
						}
					}
				}

				if matchedBranch != nil {
					caseScope := runtime.NewScope(scope)
					if matchedBranch.ParamName != "" {
						caseScope.Set(matchedBranch.ParamName, runtime.NewScalarFrame(realPayload))
					}
					resFrame, err := m.vm.executor.ExecuteInstructions(ctx, matchedBranch.Instructions, caseScope)
					if err != nil {
						executionErr = err
						return []reflect.Value{reflect.ValueOf(false)}
					}
					if resFrame != nil {
						accumulatedRows = append(accumulatedRows, resFrame.Rows...)
					}
				}
				return []reflect.Value{reflect.ValueOf(true)}
			})

			nextFunc.Call([]reflect.Value{yieldImpl})
			m.vm.decrementActiveEvents()
			if executionErr != nil {
				return nil, false, executionErr
			}
			return runtime.NewFrame(accumulatedRows), false, nil
		}

		var accumulatedRows []any
		for {
			select {
			case <-ctx.Done():
				break
			default:
			}

			results := nextFunc.Call(nil)
			if len(results) != 2 {
				break
			}
			okVal := results[1].Bool()
			if !okVal {
				break
			}

			payload := results[0].Interface()
			tag, realPayload := m.extractTagAndPayload(payload)

			var matchedBranch *ir.MatchCase
			if len(cases) == 1 {
				matchedBranch = &cases[0]
			} else {
				for idx := range cases {
					if strings.EqualFold(cases[idx].Tag, tag) || strings.EqualFold(cases[idx].Tag, "each") {
						matchedBranch = &cases[idx]
						break
					}
				}
			}

			if matchedBranch != nil {
				caseScope := runtime.NewScope(scope)
				if matchedBranch.ParamName != "" {
					caseScope.Set(matchedBranch.ParamName, runtime.NewScalarFrame(realPayload))
				}
				resFrame, err := m.vm.executor.ExecuteInstructions(ctx, matchedBranch.Instructions, caseScope)
				if err != nil {
					m.vm.decrementActiveEvents()
					return nil, false, err
				}
				if resFrame != nil {
					accumulatedRows = append(accumulatedRows, resFrame.Rows...)
				}
			}
		}

		m.vm.decrementActiveEvents()
		return runtime.NewFrame(accumulatedRows), false, nil
	}

	if nextFunc, ok := isReqReply(vVal); ok {
		m.vm.incrementActiveEvents()

		if nextFunc.Type().NumIn() == 1 {
			yieldType := nextFunc.Type().In(0)

			yieldImpl := reflect.MakeFunc(yieldType, func(args []reflect.Value) []reflect.Value {
				reqVal := args[0].Interface()
				tag, realPayload := m.extractTagAndPayload(reqVal)

				var matchedBranch *ir.MatchCase
				if len(cases) == 1 {
					matchedBranch = &cases[0]
				} else {
					for idx := range cases {
						if strings.EqualFold(cases[idx].Tag, tag) || strings.EqualFold(cases[idx].Tag, "ok") {
							matchedBranch = &cases[idx]
							break
						}
					}
				}

				var respVal any
				if matchedBranch != nil {
					caseScope := runtime.NewScope(scope)
					if matchedBranch.ParamName != "" {
						caseScope.Set(matchedBranch.ParamName, runtime.NewScalarFrame(realPayload))
					}
					resFrame, err := m.vm.executor.ExecuteInstructions(ctx, matchedBranch.Instructions, caseScope)
					if err != nil {
						panic(err)
					}
					if resFrame != nil && len(resFrame.Rows) > 0 {
						if len(resFrame.Rows) == 1 {
							respVal = resFrame.Rows[0]
						} else {
							respVal = resFrame.Rows
						}
					}
				}

				convertedResp, err := runtime.ConvertValue(respVal, yieldType.Out(0))
				if err != nil {
					panic(err)
				}
				return []reflect.Value{convertedResp}
			})

			nextFunc.Call([]reflect.Value{yieldImpl})
			return runtime.NewFrame([]any{}), false, nil
		}

		pVal := reflect.ValueOf(targetVal)
		for pVal.Kind() == reflect.Ptr {
			pVal = pVal.Elem()
		}
		if pVal.Kind() == reflect.Struct {
			cbFieldStruct := pVal.FieldByName("Callback")
			if cbFieldStruct.IsValid() && cbFieldStruct.CanSet() {
				cbType := cbFieldStruct.Type()
				reqType := cbType.In(0)
				respType := cbType.Out(0)
				_ = reqType

				callbackImpl := reflect.MakeFunc(cbType, func(args []reflect.Value) []reflect.Value {
					reqVal := args[0].Interface()
					tag, realPayload := m.extractTagAndPayload(reqVal)

					var matchedBranch *ir.MatchCase
					if len(cases) == 1 {
						matchedBranch = &cases[0]
					} else {
						for idx := range cases {
							if strings.EqualFold(cases[idx].Tag, tag) || strings.EqualFold(cases[idx].Tag, "ok") {
								matchedBranch = &cases[idx]
								break
							}
						}
					}

					var respVal any
					if matchedBranch != nil {
						caseScope := runtime.NewScope(scope)
						if matchedBranch.ParamName != "" {
							caseScope.Set(matchedBranch.ParamName, runtime.NewScalarFrame(realPayload))
						}
						resFrame, err := m.vm.executor.ExecuteInstructions(ctx, matchedBranch.Instructions, caseScope)
						if err != nil {
							panic(err)
						}
						if resFrame != nil && len(resFrame.Rows) > 0 {
							if len(resFrame.Rows) == 1 {
								respVal = resFrame.Rows[0]
							} else {
								respVal = resFrame.Rows
							}
						}
					}

					convertedResp, err := runtime.ConvertValue(respVal, respType)
					if err != nil {
						panic(err)
					}
					return []reflect.Value{convertedResp}
				})

				cbFieldStruct.Set(callbackImpl)
			}
		}
		return runtime.NewFrame([]any{}), false, nil
	}

	tag, payload := m.extractTagAndPayload(targetVal)

	var matchedBranch *ir.MatchCase
	for idx := range cases {
		if strings.EqualFold(cases[idx].Tag, tag) {
			matchedBranch = &cases[idx]
			break
		}
	}

	if matchedBranch != nil {
		caseScope := runtime.NewScope(scope)
		if matchedBranch.ParamName != "" {
			caseScope.Set(matchedBranch.ParamName, runtime.NewScalarFrame(payload))
		}
		res, err := m.vm.executor.ExecuteInstructions(ctx, matchedBranch.Instructions, caseScope)
		if err != nil {
			return nil, false, err
		}
		return res, false, nil
	}

	return nil, false, fmt.Errorf("pattern match error: unhandled case tag '%s'", tag)
}

func (m *patternMatcher) extractTagAndPayload(val any) (string, any) {
	if val == nil {
		return "nil", nil
	}

	if f, ok := val.(*runtime.Frame); ok {
		if len(f.Rows) == 1 {
			val = f.Rows[0]
		} else if len(f.Rows) == 0 {
			return "nil", nil
		}
	}

	vVal := reflect.ValueOf(val)
	for vVal.Kind() == reflect.Ptr || vVal.Kind() == reflect.Interface {
		if vVal.IsNil() {
			return "nil", nil
		}
		vVal = vVal.Elem()
	}

	// Inspecionar se possui campo Tag/Val ou tag/val
	if vVal.Kind() == reflect.Struct {
		tagField := vVal.FieldByName("Tag")
		if !tagField.IsValid() {
			tagField = vVal.FieldByName("tag")
		}
		valField := vVal.FieldByName("Val")
		if !valField.IsValid() {
			valField = vVal.FieldByName("val")
		}

		if tagField.IsValid() {
			tagVal := tagField.Interface()
			tagValVal := reflect.ValueOf(tagVal)
			if (tagValVal.Kind() == reflect.Slice || tagValVal.Kind() == reflect.Array) && tagValVal.Len() > 0 {
				tagVal = tagValVal.Index(0).Interface()
			}

			tagStr := fmt.Sprintf("%v", tagVal)
			var payload any
			if valField.IsValid() {
				payload = valField.Interface()
			}
			return tagStr, payload
		}
	}

	if mp, ok := val.(map[string]any); ok {
		tagVal, exists := mp["tag"]
		if !exists {
			tagVal, exists = mp["Tag"]
		}
		if exists {
			tagValVal := reflect.ValueOf(tagVal)
			if (tagValVal.Kind() == reflect.Slice || tagValVal.Kind() == reflect.Array) && tagValVal.Len() > 0 {
				tagVal = tagValVal.Index(0).Interface()
			}
			tagStr := fmt.Sprintf("%v", tagVal)
			payload := mp["val"]
			if payload == nil {
				payload = mp["Val"]
			}
			return tagStr, payload
		}
	}

	// Caso não estruturado, a própria representação em string vira a tag
	valVal := reflect.ValueOf(val)
	if (valVal.Kind() == reflect.Slice || valVal.Kind() == reflect.Array) && valVal.Len() > 0 {
		first := valVal.Index(0).Interface()
		return fmt.Sprintf("%v", first), val
	}

	return fmt.Sprintf("%v", val), val
}

func isSeq(v reflect.Value) (reflect.Value, bool) {
	if v.Kind() == reflect.Struct {
		typeStr := v.Type().String()
		if strings.Contains(typeStr, "SeqAccum") {
			return reflect.Value{}, false
		}
		nextField := v.FieldByName("Next")
		if nextField.IsValid() && (nextField.Kind() == reflect.Func) {
			return nextField, true
		}
	}
	if v.Kind() == reflect.Func {
		typeStr := v.Type().String()
		if strings.Contains(typeStr, "SeqAccum") {
			return reflect.Value{}, false
		}
		if strings.Contains(typeStr, "Seq") {
			return v, true
		}
	}
	return reflect.Value{}, false
}

func isSeqAccum(v reflect.Value) (reflect.Value, bool) {
	if v.Kind() == reflect.Struct {
		typeStr := v.Type().String()
		if strings.Contains(typeStr, "SeqAccum") {
			nextField := v.FieldByName("Next")
			if nextField.IsValid() && (nextField.Kind() == reflect.Func) {
				return nextField, true
			}
		}
	}
	if v.Kind() == reflect.Func {
		typeStr := v.Type().String()
		if strings.Contains(typeStr, "SeqAccum") {
			return v, true
		}
	}
	return reflect.Value{}, false
}

func isReqReply(v reflect.Value) (reflect.Value, bool) {
	if v.Kind() == reflect.Struct {
		cbField := v.FieldByName("Callback")
		if cbField.IsValid() && (cbField.Kind() == reflect.Func) {
			return cbField, true
		}
	}
	if v.Kind() == reflect.Func {
		typeStr := v.Type().String()
		if strings.Contains(typeStr, "ReqReply") {
			return v, true
		}
	}
	return reflect.Value{}, false
}
