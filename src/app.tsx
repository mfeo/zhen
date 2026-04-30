import type { TextareaRenderable } from "@opentui/core"
import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react"
import { useCallback, useRef, useState } from "react"
import { InputPane } from "./components/input-pane.js"
import { OutputPane } from "./components/output-pane.js"
import { StatusBar } from "./components/status-bar.js"
import type { Direction } from "./config.js"
import { copyToClipboard } from "./services/clipboard.js"
import { translateStream } from "./services/ollama.js"

export function App() {
  const renderer = useRenderer()
  const { width, height } = useTerminalDimensions()

  const [inputText, setInputText] = useState("")
  const [outputText, setOutputText] = useState("")
  const [isTranslating, setIsTranslating] = useState(false)
  const [statusMessage, setStatusMessage] = useState("")
  const [direction, setDirection] = useState<Direction>("zh2en")

  const abortRef = useRef<AbortController | null>(null)
  const inputRef = useRef<TextareaRenderable>(null)

  const showStatus = (msg: string, duration = 2000) => {
    setStatusMessage(msg)
    setTimeout(() => setStatusMessage(""), duration)
  }

  const handleTranslate = useCallback(async () => {
    if (!inputText.trim()) return
    if (isTranslating) {
      abortRef.current?.abort()
    }

    const ac = new AbortController()
    abortRef.current = ac
    setIsTranslating(true)
    setOutputText("")

    try {
      for await (const chunk of translateStream(inputText, direction, ac.signal)) {
        setOutputText((prev) => prev + chunk)
      }
    } catch (e: unknown) {
      if (e instanceof Error && e.name === "AbortError") return
      const msg = e instanceof Error ? e.message : String(e)
      showStatus(`Error: ${msg}`, 4000)
    } finally {
      setIsTranslating(false)
    }
  }, [inputText, isTranslating, direction])

  const handleCopy = useCallback(async () => {
    if (!outputText) {
      showStatus("Nothing to copy")
      return
    }
    try {
      await copyToClipboard(outputText)
      showStatus("✓ Copied!")
    } catch {
      showStatus("Copy failed")
    }
  }, [outputText])

  const handleToggleDirection = useCallback(() => {
    abortRef.current?.abort()
    setDirection((d) => (d === "zh2en" ? "en2zh" : "zh2en"))
    setInputText("")
    setOutputText("")
    inputRef.current?.setText("")
    setStatusMessage("")
  }, [])

  useKeyboard((key) => {
    if (key.ctrl && key.name === "t") {
      key.preventDefault()
      void handleTranslate()
    } else if (key.ctrl && key.name === "l") {
      key.preventDefault()
      handleToggleDirection()
    } else if (key.ctrl && key.name === "y") {
      key.preventDefault()
      void handleCopy()
    } else if ((key.ctrl && key.name === "q") || key.name === "escape") {
      key.preventDefault()
      abortRef.current?.abort()
      renderer.destroy()
      process.exit(0)
    }
  })

  const paneWidth = Math.floor(width / 2)
  const contentHeight = height - 1

  return (
    <box width={width} height={height} flexDirection="column">
      <box flexDirection="row" flexGrow={1}>
        <InputPane
          ref={inputRef}
          width={paneWidth}
          height={contentHeight}
          direction={direction}
          onTextChange={setInputText}
        />
        <OutputPane
          width={width - paneWidth}
          height={contentHeight}
          direction={direction}
          text={outputText}
          isTranslating={isTranslating}
        />
      </box>
      <StatusBar
        width={width}
        message={statusMessage}
        isTranslating={isTranslating}
        direction={direction}
      />
    </box>
  )
}
