import { Marked } from 'marked'
import hljs from 'highlight.js/lib/core'
import DOMPurify from 'dompurify'
import 'highlight.js/styles/github-dark.css'

import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import python from 'highlight.js/lib/languages/python'
import css from 'highlight.js/lib/languages/css'
import xml from 'highlight.js/lib/languages/xml'
import bash from 'highlight.js/lib/languages/bash'
import json from 'highlight.js/lib/languages/json'
import sql from 'highlight.js/lib/languages/sql'
import go from 'highlight.js/lib/languages/go'
import java from 'highlight.js/lib/languages/java'
import c from 'highlight.js/lib/languages/c'
import cpp from 'highlight.js/lib/languages/cpp'
import yaml from 'highlight.js/lib/languages/yaml'
import markdown from 'highlight.js/lib/languages/markdown'
import diff from 'highlight.js/lib/languages/diff'
import shell from 'highlight.js/lib/languages/shell'

hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('js', javascript)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('ts', typescript)
hljs.registerLanguage('python', python)
hljs.registerLanguage('py', python)
hljs.registerLanguage('css', css)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('shell', shell)
hljs.registerLanguage('json', json)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('go', go)
hljs.registerLanguage('java', java)
hljs.registerLanguage('c', c)
hljs.registerLanguage('cpp', cpp)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('yml', yaml)
hljs.registerLanguage('markdown', markdown)
hljs.registerLanguage('md', markdown)
hljs.registerLanguage('diff', diff)

DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A') {
    node.setAttribute('rel', 'noopener noreferrer')
    node.setAttribute('target', '_blank')
  }
})

const renderer = {
  code({ text, lang }) {
    const language = lang || 'text'
    let highlighted
    if (lang && hljs.getLanguage(lang)) {
      highlighted = hljs.highlight(text, { language: lang }).value
    } else {
      highlighted = hljs.highlightAuto(text).value
    }
    return `<div class="code-block-wrapper">
  <div class="code-block-header">
    <span class="code-lang">${language}</span>
    <button class="copy-btn" title="复制代码">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
      </svg>
      <span class="copy-text">复制</span>
    </button>
  </div>
  <pre><code class="hljs language-${language}">${highlighted}</code></pre>
</div>`
  }
}

const marked = new Marked({ renderer, gfm: true, breaks: false })

const ALLOWED_TAGS = [
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'p', 'br', 'hr', 'blockquote',
  'ul', 'ol', 'li', 'a', 'strong', 'em', 'del', 'code', 'pre',
  'table', 'thead', 'tbody', 'tr', 'th', 'td', 'img', 'span', 'div',
  'sup', 'sub', 'details', 'summary', 'button', 's'
]
const ALLOWED_ATTR = ['href', 'src', 'alt', 'title', 'class', 'target', 'rel', 'data-language', 'data-lang']

export function useMarkdown() {
  function renderMarkdown(text) {
    if (!text) return ''
    const rawHtml = marked.parse(text)
    return DOMPurify.sanitize(rawHtml, { ALLOWED_TAGS, ALLOWED_ATTR })
  }

  async function copyCode(event) {
    const btn = event.target.closest('.copy-btn')
    if (!btn) return
    const wrapper = btn.closest('.code-block-wrapper')
    if (!wrapper) return
    const code = wrapper.querySelector('code')
    if (!code) return
    try {
      await navigator.clipboard.writeText(code.textContent)
      const textEl = btn.querySelector('.copy-text')
      if (textEl) {
        textEl.textContent = '已复制'
        setTimeout(() => { textEl.textContent = '复制' }, 2000)
      }
    } catch {}
  }

  return { renderMarkdown, copyCode }
}
