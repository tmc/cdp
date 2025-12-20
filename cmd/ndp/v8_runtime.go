package main

import (
	"fmt"
	"strconv"
)

// V8Runtime provides runtime evaluation and object inspection capabilities
type V8Runtime struct {
	client *V8InspectorClient
}

// NewV8Runtime creates a new V8 runtime instance
func NewV8Runtime(client *V8InspectorClient) *V8Runtime {
	return &V8Runtime{client: client}
}

// EnableRuntime enables the Runtime domain
func (r *V8Runtime) EnableRuntime() error {
	_, err := r.client.SendCommand("Runtime.enable", nil)
	if err != nil {
		return err
	}

	r.client.runtimeEnabled = true

	if r.client.verbose {
		fmt.Println("Runtime domain enabled")
	}

	return nil
}

// DisableRuntime disables the Runtime domain
func (r *V8Runtime) DisableRuntime() error {
	_, err := r.client.SendCommand("Runtime.disable", nil)
	if err != nil {
		return err
	}

	r.client.runtimeEnabled = false
	return nil
}

// Evaluate executes JavaScript code in the global context
func (r *V8Runtime) Evaluate(expression string, options *EvaluateOptions) (*EvaluationResult, error) {
	params := map[string]interface{}{
		"expression": expression,
	}

	if options != nil {
		if options.ObjectGroup != "" {
			params["objectGroup"] = options.ObjectGroup
		}
		if options.IncludeCommandLineAPI {
			params["includeCommandLineAPI"] = true
		}
		if options.Silent {
			params["silent"] = true
		}
		if options.ContextId != 0 {
			params["contextId"] = options.ContextId
		}
		if options.ReturnByValue {
			params["returnByValue"] = true
		}
		if options.GeneratePreview {
			params["generatePreview"] = true
		}
		if options.UserGesture {
			params["userGesture"] = true
		}
		if options.AwaitPromise {
			params["awaitPromise"] = true
		}
		if options.ThrowOnSideEffect {
			params["throwOnSideEffect"] = true
		}
		if options.Timeout > 0 {
			params["timeout"] = options.Timeout
		}
		if options.DisableBreaks {
			params["disableBreaks"] = true
		}
		if options.ReplMode {
			params["replMode"] = true
		}
	}

	result, err := r.client.SendCommand("Runtime.evaluate", params)
	if err != nil {
		return nil, err
	}

	return r.parseEvaluationResult(result), nil
}

// EvaluateOptions configures runtime evaluation
type EvaluateOptions struct {
	ObjectGroup          string // Object group for object references
	IncludeCommandLineAPI bool   // Include command line API
	Silent               bool   // Don't report exceptions
	ContextId            int    // Execution context ID
	ReturnByValue        bool   // Return by value instead of object reference
	GeneratePreview      bool   // Generate object preview
	UserGesture          bool   // Treat as user gesture
	AwaitPromise         bool   // Await promises
	ThrowOnSideEffect    bool   // Throw if side effects detected
	Timeout              int    // Timeout in milliseconds
	DisableBreaks        bool   // Disable breakpoints
	ReplMode             bool   // REPL mode
}

// EvaluationResult represents the result of code evaluation
type EvaluationResult struct {
	Result       *RemoteObject         `json:"result"`
	Exception    *ExceptionDetails     `json:"exceptionDetails,omitempty"`
	RawResult    map[string]interface{} `json:"-"`
}

// RemoteObject represents a remote JavaScript object
type RemoteObject struct {
	Type                string                 `json:"type"`
	Subtype             string                 `json:"subtype,omitempty"`
	ClassName           string                 `json:"className,omitempty"`
	Value               interface{}            `json:"value,omitempty"`
	UnserializableValue string                 `json:"unserializableValue,omitempty"`
	Description         string                 `json:"description,omitempty"`
	ObjectID            string                 `json:"objectId,omitempty"`
	Preview             map[string]interface{} `json:"preview,omitempty"`
}

// ExceptionDetails provides details about runtime exceptions
type ExceptionDetails struct {
	ExceptionID   int                    `json:"exceptionId"`
	Text          string                 `json:"text"`
	LineNumber    int                    `json:"lineNumber"`
	ColumnNumber  int                    `json:"columnNumber"`
	ScriptID      string                 `json:"scriptId,omitempty"`
	URL           string                 `json:"url,omitempty"`
	StackTrace    map[string]interface{} `json:"stackTrace,omitempty"`
	Exception     *RemoteObject          `json:"exception,omitempty"`
	ExecutionContextID int               `json:"executionContextId,omitempty"`
}

// GetProperties retrieves properties of a remote object
func (r *V8Runtime) GetProperties(objectID string, ownProperties bool, accessorPropertiesOnly bool) ([]PropertyDescriptor, error) {
	params := map[string]interface{}{
		"objectId":               objectID,
		"ownProperties":          ownProperties,
		"accessorPropertiesOnly": accessorPropertiesOnly,
	}

	result, err := r.client.SendCommand("Runtime.getProperties", params)
	if err != nil {
		return nil, err
	}

	var properties []PropertyDescriptor
	if props, ok := result["result"].([]interface{}); ok {
		for _, prop := range props {
			if propMap, ok := prop.(map[string]interface{}); ok {
				properties = append(properties, r.parsePropertyDescriptor(propMap))
			}
		}
	}

	return properties, nil
}

// PropertyDescriptor represents an object property
type PropertyDescriptor struct {
	Name         string        `json:"name"`
	Value        *RemoteObject `json:"value,omitempty"`
	Writable     bool          `json:"writable,omitempty"`
	Get          *RemoteObject `json:"get,omitempty"`
	Set          *RemoteObject `json:"set,omitempty"`
	Configurable bool          `json:"configurable"`
	Enumerable   bool          `json:"enumerable"`
	WasThrown    bool          `json:"wasThrown,omitempty"`
	IsOwn        bool          `json:"isOwn,omitempty"`
	Symbol       *RemoteObject `json:"symbol,omitempty"`
}

// CallFunctionOn calls a function on a remote object
func (r *V8Runtime) CallFunctionOn(objectID, functionDeclaration string, arguments []CallArgument, silent bool) (*EvaluationResult, error) {
	params := map[string]interface{}{
		"functionDeclaration": functionDeclaration,
		"silent":              silent,
	}

	if objectID != "" {
		params["objectId"] = objectID
	}

	if len(arguments) > 0 {
		var args []map[string]interface{}
		for _, arg := range arguments {
			argMap := make(map[string]interface{})
			if arg.Value != nil {
				argMap["value"] = arg.Value
			}
			if arg.UnserializableValue != "" {
				argMap["unserializableValue"] = arg.UnserializableValue
			}
			if arg.ObjectID != "" {
				argMap["objectId"] = arg.ObjectID
			}
			args = append(args, argMap)
		}
		params["arguments"] = args
	}

	result, err := r.client.SendCommand("Runtime.callFunctionOn", params)
	if err != nil {
		return nil, err
	}

	return r.parseEvaluationResult(result), nil
}

// CallArgument represents an argument for function calls
type CallArgument struct {
	Value               interface{} `json:"value,omitempty"`
	UnserializableValue string      `json:"unserializableValue,omitempty"`
	ObjectID            string      `json:"objectId,omitempty"`
}

// ReleaseObject releases a remote object
func (r *V8Runtime) ReleaseObject(objectID string) error {
	params := map[string]interface{}{
		"objectId": objectID,
	}

	_, err := r.client.SendCommand("Runtime.releaseObject", params)
	return err
}

// ReleaseObjectGroup releases all objects in an object group
func (r *V8Runtime) ReleaseObjectGroup(objectGroup string) error {
	params := map[string]interface{}{
		"objectGroup": objectGroup,
	}

	_, err := r.client.SendCommand("Runtime.releaseObjectGroup", params)
	return err
}

// GetExecutionContexts retrieves available execution contexts
func (r *V8Runtime) GetExecutionContexts() ([]ExecutionContextDescription, error) {
	result, err := r.client.SendCommand("Runtime.getExecutionContexts", nil)
	if err != nil {
		return nil, err
	}

	var contexts []ExecutionContextDescription
	if ctxs, ok := result["contexts"].([]interface{}); ok {
		for _, ctx := range ctxs {
			if ctxMap, ok := ctx.(map[string]interface{}); ok {
				contexts = append(contexts, r.parseExecutionContext(ctxMap))
			}
		}
	}

	return contexts, nil
}

// ExecutionContextDescription describes an execution context
type ExecutionContextDescription struct {
	ID         int                    `json:"id"`
	Origin     string                 `json:"origin"`
	Name       string                 `json:"name"`
	AuxData    map[string]interface{} `json:"auxData,omitempty"`
}

// CompileScript compiles a script and returns script ID
func (r *V8Runtime) CompileScript(expression, sourceURL string, persistScript bool, executionContextID int) (string, error) {
	params := map[string]interface{}{
		"expression":     expression,
		"sourceURL":      sourceURL,
		"persistScript":  persistScript,
	}

	if executionContextID > 0 {
		params["executionContextId"] = executionContextID
	}

	result, err := r.client.SendCommand("Runtime.compileScript", params)
	if err != nil {
		return "", err
	}

	if scriptID, ok := result["scriptId"].(string); ok {
		return scriptID, nil
	}

	return "", fmt.Errorf("failed to compile script")
}

// RunScript runs a compiled script
func (r *V8Runtime) RunScript(scriptID string, executionContextID int, objectGroup string, silent bool, includeCommandLineAPI bool, returnByValue bool, generatePreview bool, awaitPromise bool) (*EvaluationResult, error) {
	params := map[string]interface{}{
		"scriptId": scriptID,
	}

	if executionContextID > 0 {
		params["executionContextId"] = executionContextID
	}
	if objectGroup != "" {
		params["objectGroup"] = objectGroup
	}
	if silent {
		params["silent"] = silent
	}
	if includeCommandLineAPI {
		params["includeCommandLineAPI"] = includeCommandLineAPI
	}
	if returnByValue {
		params["returnByValue"] = returnByValue
	}
	if generatePreview {
		params["generatePreview"] = generatePreview
	}
	if awaitPromise {
		params["awaitPromise"] = awaitPromise
	}

	result, err := r.client.SendCommand("Runtime.runScript", params)
	if err != nil {
		return nil, err
	}

	return r.parseEvaluationResult(result), nil
}

// Utility methods for parsing CDP responses

func (r *V8Runtime) parseEvaluationResult(result map[string]interface{}) *EvaluationResult {
	evalResult := &EvaluationResult{
		RawResult: result,
	}

	if resultObj, ok := result["result"].(map[string]interface{}); ok {
		evalResult.Result = r.parseRemoteObject(resultObj)
	}

	if exception, ok := result["exceptionDetails"].(map[string]interface{}); ok {
		evalResult.Exception = r.parseExceptionDetails(exception)
	}

	return evalResult
}

func (r *V8Runtime) parseRemoteObject(obj map[string]interface{}) *RemoteObject {
	remote := &RemoteObject{}

	if typ, ok := obj["type"].(string); ok {
		remote.Type = typ
	}
	if subtype, ok := obj["subtype"].(string); ok {
		remote.Subtype = subtype
	}
	if className, ok := obj["className"].(string); ok {
		remote.ClassName = className
	}
	if value, ok := obj["value"]; ok {
		remote.Value = value
	}
	if unserializableValue, ok := obj["unserializableValue"].(string); ok {
		remote.UnserializableValue = unserializableValue
	}
	if description, ok := obj["description"].(string); ok {
		remote.Description = description
	}
	if objectID, ok := obj["objectId"].(string); ok {
		remote.ObjectID = objectID
	}
	if preview, ok := obj["preview"].(map[string]interface{}); ok {
		remote.Preview = preview
	}

	return remote
}

func (r *V8Runtime) parseExceptionDetails(exception map[string]interface{}) *ExceptionDetails {
	details := &ExceptionDetails{}

	if exceptionID, ok := exception["exceptionId"].(float64); ok {
		details.ExceptionID = int(exceptionID)
	}
	if text, ok := exception["text"].(string); ok {
		details.Text = text
	}
	if lineNumber, ok := exception["lineNumber"].(float64); ok {
		details.LineNumber = int(lineNumber)
	}
	if columnNumber, ok := exception["columnNumber"].(float64); ok {
		details.ColumnNumber = int(columnNumber)
	}
	if scriptID, ok := exception["scriptId"].(string); ok {
		details.ScriptID = scriptID
	}
	if url, ok := exception["url"].(string); ok {
		details.URL = url
	}
	if stackTrace, ok := exception["stackTrace"].(map[string]interface{}); ok {
		details.StackTrace = stackTrace
	}
	if exceptionObj, ok := exception["exception"].(map[string]interface{}); ok {
		details.Exception = r.parseRemoteObject(exceptionObj)
	}
	if executionContextID, ok := exception["executionContextId"].(float64); ok {
		details.ExecutionContextID = int(executionContextID)
	}

	return details
}

func (r *V8Runtime) parsePropertyDescriptor(prop map[string]interface{}) PropertyDescriptor {
	desc := PropertyDescriptor{}

	if name, ok := prop["name"].(string); ok {
		desc.Name = name
	}
	if value, ok := prop["value"].(map[string]interface{}); ok {
		desc.Value = r.parseRemoteObject(value)
	}
	if writable, ok := prop["writable"].(bool); ok {
		desc.Writable = writable
	}
	if get, ok := prop["get"].(map[string]interface{}); ok {
		desc.Get = r.parseRemoteObject(get)
	}
	if set, ok := prop["set"].(map[string]interface{}); ok {
		desc.Set = r.parseRemoteObject(set)
	}
	if configurable, ok := prop["configurable"].(bool); ok {
		desc.Configurable = configurable
	}
	if enumerable, ok := prop["enumerable"].(bool); ok {
		desc.Enumerable = enumerable
	}
	if wasThrown, ok := prop["wasThrown"].(bool); ok {
		desc.WasThrown = wasThrown
	}
	if isOwn, ok := prop["isOwn"].(bool); ok {
		desc.IsOwn = isOwn
	}
	if symbol, ok := prop["symbol"].(map[string]interface{}); ok {
		desc.Symbol = r.parseRemoteObject(symbol)
	}

	return desc
}

func (r *V8Runtime) parseExecutionContext(ctx map[string]interface{}) ExecutionContextDescription {
	desc := ExecutionContextDescription{}

	if id, ok := ctx["id"].(float64); ok {
		desc.ID = int(id)
	}
	if origin, ok := ctx["origin"].(string); ok {
		desc.Origin = origin
	}
	if name, ok := ctx["name"].(string); ok {
		desc.Name = name
	}
	if auxData, ok := ctx["auxData"].(map[string]interface{}); ok {
		desc.AuxData = auxData
	}

	return desc
}

// FormatValue formats a remote object value for display
func (r *V8Runtime) FormatValue(obj *RemoteObject) string {
	if obj == nil {
		return "null"
	}

	if obj.UnserializableValue != "" {
		return obj.UnserializableValue
	}

	if obj.Value != nil {
		switch obj.Type {
		case "string":
			return fmt.Sprintf("\"%s\"", obj.Value)
		case "number":
			if f, ok := obj.Value.(float64); ok {
				if f == float64(int64(f)) {
					return strconv.FormatInt(int64(f), 10)
				}
			}
			return fmt.Sprintf("%v", obj.Value)
		case "boolean":
			return fmt.Sprintf("%v", obj.Value)
		default:
			return fmt.Sprintf("%v", obj.Value)
		}
	}

	if obj.Description != "" {
		return obj.Description
	}

	return fmt.Sprintf("[%s]", obj.Type)
}