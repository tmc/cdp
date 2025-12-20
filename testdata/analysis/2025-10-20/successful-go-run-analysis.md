# Analysis of Successful Go Computer-Use-Agent Run

## Summary

This analysis examines the API traffic captured from a **successful** Go-based computer-use-agent run on October 20, 2025. The Proxyman log file contains the complete request/response data for interactions with the Gemini API.

### Key Findings

- **Total API Requests**: 2
- **API Endpoint**: `gemini-2.5-computer-use-preview-10-2025:generateContent`
- **Task**: "search about metaprompting"

---

## Request Structure Analysis

### Request 1: Initial User Message

**Contents Array** (1 turn):
- **Turn 1 (user)**:
  - Part 1: Text - `"search about metaprompting"`
  - Part 2: **Screenshot (inlineData)** - 95,396 bytes base64-encoded PNG
    - MIME type: `image/png`

**Tool Configuration**:
```json
{
  "computerUse": {
    "environment": "ENVIRONMENT_BROWSER"
  }
}
```

**Response**:
- Model called `open_web_browser` function
- Token usage: 2,014 prompt tokens (208 text + 1,806 image)

---

### Request 2: Conversation with Function Response

**Contents Array** (3 turns):

#### Turn 1 (user)
- Part 1: Text - `"search about metaprompting"`
- Part 2: **Screenshot (inlineData)** - 95,396 bytes

#### Turn 2 (model)
- Part 1: **FunctionCall** - `open_web_browser`

#### Turn 3 (user) - **CRITICAL STRUCTURE**
- Part 1: **FunctionResponse** for `open_web_browser`
  - **Contains nested `parts` array with the screenshot**:
  ```json
  {
    "functionResponse": {
      "name": "open_web_browser",
      "parts": [
        {
          "inlineData": {
            "data": "iVBORw0KGgo... [base64 data]",
            "mimeType": "image/png"
          }
        }
      ]
    }
  }
  ```

This is the **CORRECT** structure that works!

**Response**:
- Model provided reasoning and called `type_text_at` function
- Token usage: 3,845 prompt tokens (233 text + 3,612 image)

---

## Correct Screenshot Embedding Pattern

The key insight from this successful run is that **screenshots must be embedded inside the FunctionResponse**:

```
User Turn (FunctionResponse)
└── parts[]
    └── functionResponse
        └── name: "function_name"
        └── parts[]  ← SCREENSHOTS GO HERE
            └── inlineData
                └── data: "<base64>"
                └── mimeType: "image/png"
```

**NOT** at the top level of the user turn like this (INCORRECT):
```
User Turn
└── parts[]
    ├── functionResponse
    └── inlineData  ← WRONG LOCATION
```

---

## Comparison with Previous Broken Structure

### What Was Wrong Before

In previous attempts, screenshots were likely placed:
1. As separate parts alongside the FunctionResponse
2. At the top level of content.parts[] instead of inside functionResponse.parts[]
3. Missing entirely from FunctionResponse

### What Works Now

The **working structure** embeds screenshots **inside** the FunctionResponse's own parts array:

```json
{
  "parts": [
    {
      "functionResponse": {
        "name": "open_web_browser",
        "parts": [
          {
            "inlineData": {
              "data": "<base64-encoded-screenshot>",
              "mimeType": "image/png"
            }
          }
        ]
      }
    }
  ],
  "role": "user"
}
```

---

## Conversation Flow

1. **User** sends initial prompt + screenshot
2. **Model** responds with function call (`open_web_browser`)
3. **User** sends FunctionResponse with:
   - Function name: `open_web_browser`
   - **Screenshot embedded in FunctionResponse.parts[]**
4. **Model** processes screenshot and responds with next action

---

## Token Usage

### Request 1:
- Prompt: 2,014 tokens (208 text, 1,806 image)
- Response: 12 tokens

### Request 2:
- Prompt: 3,845 tokens (233 text, 3,612 image)
- Response: 104 tokens

Note: Each screenshot contributes ~1,800 tokens to the request.

---

## Key Takeaways

1. **Screenshots MUST be inside FunctionResponse.parts[]** - This is the critical difference between working and broken implementations

2. **The structure is nested** - FunctionResponse has its own parts array where multimodal content (images) should be placed

3. **Conversation history order is maintained** - user → model → user pattern with FunctionResponses in the user turns

4. **Tool configuration uses ENVIRONMENT_BROWSER** - Specifies the computer use environment

---

## File Location

Source log file:
```
/Volumes/tmc/go/src/github.com/tmc/misc/chrome-to-har/testdata/generativelanguage.googleapis.com_10-20-2025-00-21-02.proxymanlogv2
```

---

## Conclusion

The successful Go implementation correctly embeds screenshots **inside** the FunctionResponse's parts array, not as separate sibling parts. This nested structure allows the API to properly associate the screenshot with the specific function response it represents.

Any implementation must ensure:
- FunctionResponse has a `parts` field
- Screenshots (inlineData) are placed in `functionResponse.parts[]`
- The overall conversation maintains user → model → user turn structure
