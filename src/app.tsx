import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react"
import { useCallback, useRef, useState } from "react"
import { InputPane } from "./components/input-pane.js"
import { OutputPane } from "./components/output-pane.js"
import { StatusBar } from "./components/status-bar.js"
import { copyToClipboard } from "./services/clipboard.js"
import { translateStream } from "./services/ollama.js"

export function App() {
  const renderer = useRenderer()
  const { width, height } = useTerminalDimensions()

  const [chineseText, setChineseText] = useState("")
  const [englishText, setEnglishText] = useState("")
  const [isTranslating, setIsTranslating] = useState(false)
  const [statusMessage, setStatusMessage] = useState("")

  const abortRef = useRef<AbortController | null>(null)

  const showStatus = (msg: string, duration = 2000) => {
    setStatusMessage(msg)
    setTimeout(() => setStatusMessage(""), duration)
  }

  const handleTranslate = useCallback(async () => {
    if (!chineseText.trim()) return
    if (isTranslating) {
      abortRef.current?.abort()
    }

    const ac = new AbortController()
    abortRef.current = ac
    setIsTranslating(true)
    setEnglishText("")

    try {
      for await (const chunk of translateStream(chineseText, ac.signal)) {
        setEnglishText((prev) => prev + chunk)
      }
    } catch (e: unknown) {
      if (e instanceof Error && e.name === "AbortError") return
      const msg = e instanceof Error ? e.message : String(e)
      showStatus(`Error: ${msg}`, 4000)
    } finally {
      setIsTranslating(false)
    }
  }, [chineseText, isTranslating])

  const handleCopy = useCallback(async () => {
    if (!englishText) {
      showStatus("Nothing to copy")
      return
    }
    try {
      await copyToClipboard(englishText)
      showStatus("✓ Copied!")
    } catch {
      showStatus("Copy failed")
    }
  }, [englishText])

  useKeyboard((key) => {
    if (key.ctrl && key.name === "t") {
      key.preventDefault()
      void handleTranslate()
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
        <InputPane width={paneWidth} height={contentHeight} onTextChange={setChineseText} />
        <OutputPane
          width={width - paneWidth}
          height={contentHeight}
          text={englishText}
          isTranslating={isTranslating}
        />
      </box>
      <StatusBar width={width} message={statusMessage} isTranslating={isTranslating} />
    </box>
  )
}
