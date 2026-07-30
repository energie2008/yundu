// Lightweight i18n engine for user-web.
// Source language is zh-CN (strings in code). Other languages are translated
// at the DOM level via dictionaries keyed by the original Chinese text.
// - Exact match on normalized text
// - Numeric tokenization: "剩余 3 天" -> key "剩余 {0} 天"
// - Prefix match for date/value suffix strings: "创建于 2026-07-31"
// - Symbol stripping: "🔄 续费订阅" -> key "续费订阅"
import React, { createContext, useContext, useEffect, useState } from 'react';
import zhTW from '../locales/zh-TW';
import ru from '../locales/ru';
import ko from '../locales/ko';
import vi from '../locales/vi';
import th from '../locales/th';

export const LOCALES = [
  { code: 'zh-CN', short: 'CN', label: '简体中文' },
  { code: 'zh-TW', short: 'TW', label: '繁體中文' },
  { code: 'ru', short: 'RU', label: 'Русский' },
  { code: 'ko', short: 'KR', label: '한국어' },
  { code: 'vi', short: 'VN', label: 'Tiếng Việt' },
  { code: 'th', short: 'TH', label: 'ไทย' },
] as const;

export type LangCode = (typeof LOCALES)[number]['code'];

const DICTS: Record<string, Record<string, string>> = {
  'zh-TW': zhTW,
  ru,
  ko,
  vi,
  th,
};

const STORAGE_KEY = 'yundu-lang';

// Prefixes whose remainder is dynamic (dates, amounts); dict stores the prefix.
const PREFIXES = [
  '创建时间：',
  '更新时间：',
  '可用余额：',
  '当前可提现余额：',
  '创建于',
  '发布于',
  '注册于',
  '最后回复',
  '请求失败',
];

const CJK_RE = /[\u4e00-\u9fff]/;
const NUM_TOKEN_RE = /[¥￥$]?\d+(?:[.,]\d+)?/g;
// leading/trailing symbols (emoji, arrows, bullets) that wrap a translatable core
const LEAD_SYM_RE = /^[^\u4e00-\u9fffA-Za-z0-9]+/;
const TRAIL_SYM_RE = /[^\u4e00-\u9fffA-Za-z0-9？！。？!.]+$/;

let currentLang: LangCode = (() => {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v && LOCALES.some(l => l.code === v)) return v as LangCode;
  } catch { /* ignore */ }
  return 'zh-CN';
})();

function normalize(s: string): string {
  return s.replace(/\s+/g, ' ').trim();
}

function lookupCore(dict: Record<string, string>, key: string): string | null {
  // 1. exact
  const exact = dict[key];
  if (exact) return exact;
  // 2. numeric tokenization
  const tokens: string[] = [];
  const pattern = key.replace(NUM_TOKEN_RE, m => {
    tokens.push(m);
    return '{' + (tokens.length - 1) + '}';
  });
  if (tokens.length > 0) {
    const hit = dict[pattern];
    if (hit) return hit.replace(/\{(\d+)\}/g, (_, i) => tokens[Number(i)] ?? '');
  }
  // 3. prefix match (date/amount suffix)
  for (const p of PREFIXES) {
    if (key.startsWith(p) && key.length > p.length) {
      const hit = dict[p];
      if (hit) return hit + key.slice(p.length);
    }
  }
  return null;
}

function translateString(raw: string, lang: LangCode = currentLang): string | null {
  if (lang === 'zh-CN') return null;
  const dict = DICTS[lang];
  if (!dict) return null;
  if (!CJK_RE.test(raw)) return null;
  const key = normalize(raw);
  if (!key) return null;

  let result = lookupCore(dict, key);
  if (result === null) {
    // 4. strip wrapping symbols (emoji / bullets / arrows) and retry
    const lead = key.match(LEAD_SYM_RE)?.[0] ?? '';
    let core = key.slice(lead.length);
    const trail = core.match(TRAIL_SYM_RE)?.[0] ?? '';
    core = trail ? core.slice(0, core.length - trail.length) : core;
    if (core && core !== key) {
      const hit = lookupCore(dict, core);
      if (hit !== null) result = lead + hit + trail;
    }
  }
  if (result === null) return null;
  // preserve original leading/trailing whitespace
  const ws = raw.match(/^\s*/)![0];
  const we = raw.match(/\s*$/)![0];
  return ws + result + we;
}

/** Translate a plain JS string (for confirm()/clipboard text etc.). */
export function t(s: string): string {
  return translateString(s) ?? s;
}

// ---------------------------------------------------------------------------
// DOM translation engine
// ---------------------------------------------------------------------------
const TEXT_ORIG = new WeakMap<Text, string>();
const ATTR_ORIG = new WeakMap<Element, Record<string, string>>();
const ATTRS = ['placeholder', 'title', 'aria-label', 'alt'];

function applyTextNode(node: Text) {
  let orig = TEXT_ORIG.get(node);
  if (orig === undefined) {
    orig = node.data;
    if (!CJK_RE.test(orig)) return;
    TEXT_ORIG.set(node, orig);
  }
  const out = currentLang === 'zh-CN' ? orig : translateString(orig) ?? orig;
  if (node.data !== out) node.data = out;
}

function refreshTextNode(node: Text) {
  // Called when node data changed externally (React re-render).
  const orig = TEXT_ORIG.get(node);
  if (orig !== undefined) {
    const expected = currentLang === 'zh-CN' ? orig : translateString(orig) ?? orig;
    if (node.data === expected) return; // our own write
    TEXT_ORIG.delete(node);
  }
  applyTextNode(node);
}

function applyElementAttrs(el: Element) {
  let bag = ATTR_ORIG.get(el);
  for (const a of ATTRS) {
    const cur = el.getAttribute(a);
    if (cur === null) continue;
    let orig = bag?.[a];
    if (orig === undefined) {
      if (!CJK_RE.test(cur)) continue;
      orig = cur;
      if (!bag) {
        bag = {};
        ATTR_ORIG.set(el, bag);
      }
      bag[a] = orig;
    } else {
      const expected = currentLang === 'zh-CN' ? orig : translateString(orig) ?? orig;
      if (cur !== expected && cur !== orig) {
        // React updated the attribute to a new source value
        bag![a] = cur;
        orig = cur;
      }
    }
    const out = currentLang === 'zh-CN' ? orig : translateString(orig) ?? orig;
    if (el.getAttribute(a) !== out) el.setAttribute(a, out);
  }
}

function walk(root: Node) {
  if (root.nodeType === Node.TEXT_NODE) {
    applyTextNode(root as Text);
    return;
  }
  if (root.nodeType !== Node.ELEMENT_NODE && root.nodeType !== Node.DOCUMENT_NODE) return;
  const el = root as Element;
  if (el.nodeType === Node.ELEMENT_NODE) {
    const tag = el.tagName;
    if (tag === 'SCRIPT' || tag === 'STYLE') return;
    applyElementAttrs(el);
  }
  const tw = document.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT, {
    acceptNode(n) {
      if (n.nodeType === Node.ELEMENT_NODE) {
        const t = (n as Element).tagName;
        return t === 'SCRIPT' || t === 'STYLE' ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_SKIP;
      }
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  let n: Node | null;
  while ((n = tw.nextNode())) applyTextNode(n as Text);
  if (el.querySelectorAll) {
    el.querySelectorAll('[placeholder],[title],[aria-label],[alt]').forEach(applyElementAttrs);
  }
}

let observerStarted = false;

function startObserver() {
  if (observerStarted) return;
  observerStarted = true;
  const observer = new MutationObserver(mutations => {
    for (const m of mutations) {
      if (m.type === 'characterData') {
        refreshTextNode(m.target as Text);
      } else if (m.type === 'childList') {
        m.addedNodes.forEach(n => walk(n));
      } else if (m.type === 'attributes') {
        applyElementAttrs(m.target as Element);
      }
    }
  });
  observer.observe(document.body, {
    subtree: true,
    childList: true,
    characterData: true,
    attributes: true,
    attributeFilter: ATTRS,
  });
}

function applyAll() {
  walk(document.body);
}

// ---------------------------------------------------------------------------
// React context
// ---------------------------------------------------------------------------
interface I18nContextType {
  lang: LangCode;
  setLang: (l: LangCode) => void;
}

const I18nContext = createContext<I18nContextType>({
  lang: 'zh-CN',
  setLang: () => {},
});

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [lang, setLangState] = useState<LangCode>(currentLang);

  useEffect(() => {
    document.documentElement.lang = currentLang;
    startObserver();
    applyAll();
  }, []);

  const setLang = (l: LangCode) => {
    if (l === currentLang) return;
    currentLang = l;
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch { /* ignore */ }
    document.documentElement.lang = l;
    setLangState(l);
    applyAll();
  };

  return <I18nContext.Provider value={{ lang, setLang }}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  return useContext(I18nContext);
}
