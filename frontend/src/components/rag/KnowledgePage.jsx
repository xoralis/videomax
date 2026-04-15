import { useRef, useState } from 'react';
import { ingestFile, ingestText, searchKnowledge } from '../../services/ragService';

// ── 语义检索 Tab ─────────────────────────────────────────────
function SearchTab() {
  const [query, setQuery] = useState('');
  const [topK, setTopK] = useState(3);
  const [results, setResults] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSearch = async (e) => {
    e.preventDefault();
    const q = query.trim();
    if (!q) return;
    setLoading(true);
    setError('');
    setResults(null);
    try {
      const data = await searchKnowledge(q, topK);
      setResults(data.results || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <form onSubmit={handleSearch} className="flex flex-col sm:flex-row gap-3">
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="输入检索内容…"
          className="flex-1 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 px-4 py-2.5 text-sm focus:outline-none focus:border-cyan-500 transition-colors"
        />
        <select
          value={topK}
          onChange={(e) => setTopK(Number(e.target.value))}
          className="w-full sm:w-28 rounded-lg bg-slate-800 border border-slate-700 text-slate-300 px-3 py-2.5 text-sm focus:outline-none focus:border-cyan-500 transition-colors"
        >
          {[1, 3, 5, 10].map((n) => (
            <option key={n} value={n}>
              Top-{n}
            </option>
          ))}
        </select>
        <button
          type="submit"
          disabled={loading || !query.trim()}
          className="rounded-lg bg-cyan-600 hover:bg-cyan-500 disabled:opacity-40 text-white text-sm font-medium px-6 py-2.5 transition-colors"
        >
          {loading ? '检索中…' : '检索'}
        </button>
      </form>

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-red-400 text-sm">
          {error}
        </div>
      )}

      {results !== null && (
        <div className="space-y-3">
          {results.length === 0 ? (
            <p className="text-slate-500 text-sm text-center py-8">未找到相关内容</p>
          ) : (
            results.map((item, idx) => (
              <div
                key={item.id || idx}
                className="rounded-lg border border-slate-700 bg-slate-800/60 p-4 space-y-2"
              >
                <div className="flex items-start justify-between gap-3">
                  <span className="text-xs font-semibold text-cyan-400 shrink-0">#{idx + 1}</span>
                  {item.metadata?.source && (
                    <span className="text-xs bg-slate-700 text-slate-300 rounded px-2 py-0.5 shrink-0">
                      {item.metadata.source}
                    </span>
                  )}
                </div>
                <p className="text-slate-300 text-sm leading-relaxed whitespace-pre-wrap break-words">
                  {item.content}
                </p>
                {item.metadata && Object.keys(item.metadata).filter((k) => k !== 'source').length > 0 && (
                  <div className="flex flex-wrap gap-2 pt-1">
                    {Object.entries(item.metadata)
                      .filter(([k]) => k !== 'source')
                      .map(([k, v]) => (
                        <span key={k} className="text-xs text-slate-500">
                          {k}: {String(v)}
                        </span>
                      ))}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}

// ── 上传文档 Tab ─────────────────────────────────────────────
const ACCEPT_EXTS = '.txt,.md,.markdown,.pdf';

function UploadTab() {
  const fileInputRef = useRef(null);
  const [dragging, setDragging] = useState(false);
  const [selectedFile, setSelectedFile] = useState(null);
  const [source, setSource] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');

  const pickFile = (file) => {
    if (!file) return;
    setSelectedFile(file);
    setResult(null);
    setError('');
  };

  const handleDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    const file = e.dataTransfer.files[0];
    pickFile(file);
  };

  const handleUpload = async () => {
    if (!selectedFile) return;
    setLoading(true);
    setError('');
    setResult(null);
    try {
      const data = await ingestFile(selectedFile, source.trim());
      setResult(data);
      setSelectedFile(null);
      setSource('');
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-5">
      {/* 拖拽区 */}
      <div
        onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
        onDragLeave={() => setDragging(false)}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
        className={`relative flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed cursor-pointer transition-colors min-h-[160px] px-6 py-10
          ${dragging
            ? 'border-cyan-500 bg-cyan-500/5'
            : 'border-slate-700 bg-slate-800/40 hover:border-slate-600'
          }`}
      >
        <input
          ref={fileInputRef}
          type="file"
          accept={ACCEPT_EXTS}
          className="hidden"
          onChange={(e) => pickFile(e.target.files[0])}
        />
        <svg className="w-10 h-10 text-slate-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
            d="M12 16v-8m0 0-3 3m3-3 3 3M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1" />
        </svg>
        {selectedFile ? (
          <div className="text-center">
            <p className="text-white text-sm font-medium">{selectedFile.name}</p>
            <p className="text-slate-500 text-xs mt-1">
              {(selectedFile.size / 1024).toFixed(1)} KB
            </p>
          </div>
        ) : (
          <div className="text-center">
            <p className="text-slate-400 text-sm">拖拽文件到此处，或点击选择</p>
            <p className="text-slate-600 text-xs mt-1">支持 .txt .md .markdown .pdf</p>
          </div>
        )}
      </div>

      {/* 来源标签 */}
      <div>
        <label className="block text-slate-400 text-xs mb-1.5">来源标签（可选）</label>
        <input
          type="text"
          value={source}
          onChange={(e) => setSource(e.target.value)}
          placeholder="例如：产品文档 / API 手册"
          className="w-full rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 px-4 py-2.5 text-sm focus:outline-none focus:border-cyan-500 transition-colors"
        />
      </div>

      {/* 上传按钮 */}
      <button
        onClick={handleUpload}
        disabled={!selectedFile || loading}
        className="w-full rounded-lg bg-cyan-600 hover:bg-cyan-500 disabled:opacity-40 text-white text-sm font-medium py-2.5 transition-colors"
      >
        {loading ? '入库中，请稍候…' : '开始入库'}
      </button>

      {/* 错误 */}
      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-red-400 text-sm">
          {error}
        </div>
      )}

      {/* 成功 */}
      {result && (
        <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-emerald-400 text-sm">
          ✓ 入库成功，共写入 <strong>{result.ingested}</strong> 个片段
        </div>
      )}
    </div>
  );
}

// ── 文本直接入库 Tab ──────────────────────────────────────────
function TextIngestTab() {
  const [id, setId] = useState('');
  const [content, setContent] = useState('');
  const [source, setSource] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');

  const handleSubmit = async (e) => {
    e.preventDefault();
    const trimmed = content.trim();
    if (!trimmed) return;
    setLoading(true);
    setError('');
    setResult(null);
    try {
      const doc = {
        id: id.trim() || crypto.randomUUID(),
        content: trimmed,
        metadata: source.trim() ? { source: source.trim() } : {},
      };
      const data = await ingestText([doc]);
      setResult(data);
      setId('');
      setContent('');
      setSource('');
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label className="block text-slate-400 text-xs mb-1.5">文档 ID（可选，留空自动生成）</label>
        <input
          type="text"
          value={id}
          onChange={(e) => setId(e.target.value)}
          placeholder="my-doc-001"
          className="w-full rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 px-4 py-2.5 text-sm focus:outline-none focus:border-cyan-500 transition-colors"
        />
      </div>
      <div>
        <label className="block text-slate-400 text-xs mb-1.5">文档内容 *</label>
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          rows={7}
          placeholder="在此粘贴或输入文本内容…"
          className="w-full rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 px-4 py-2.5 text-sm focus:outline-none focus:border-cyan-500 transition-colors resize-none"
        />
      </div>
      <div>
        <label className="block text-slate-400 text-xs mb-1.5">来源标签（可选）</label>
        <input
          type="text"
          value={source}
          onChange={(e) => setSource(e.target.value)}
          placeholder="例如：wiki"
          className="w-full rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 px-4 py-2.5 text-sm focus:outline-none focus:border-cyan-500 transition-colors"
        />
      </div>

      <button
        type="submit"
        disabled={!content.trim() || loading}
        className="w-full rounded-lg bg-cyan-600 hover:bg-cyan-500 disabled:opacity-40 text-white text-sm font-medium py-2.5 transition-colors"
      >
        {loading ? '入库中…' : '提交入库'}
      </button>

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-red-400 text-sm">
          {error}
        </div>
      )}
      {result && (
        <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-emerald-400 text-sm">
          ✓ 入库成功，共写入 <strong>{result.ingested}</strong> 个片段
        </div>
      )}
    </form>
  );
}

// ── 主页面 ──────────────────────────────────────────────────
const TABS = [
  { key: 'search', label: '语义检索' },
  { key: 'upload', label: '上传文档' },
  { key: 'text', label: '文本入库' },
];

export default function KnowledgePage() {
  const [tab, setTab] = useState('search');

  return (
    <main className="w-full max-w-3xl mx-auto pt-8 pb-24 px-4 sm:px-6">
      {/* 页面标题 */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-white">知识库</h1>
        <p className="text-slate-500 text-sm mt-1">向量检索 · 文档管理</p>
      </div>

      {/* Tab 栏 */}
      <div className="flex gap-1 border-b border-slate-800 mb-6">
        {TABS.map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setTab(key)}
            className={`px-4 py-2 text-sm font-medium transition-colors rounded-t
              ${tab === key
                ? 'text-cyan-400 border-b-2 border-cyan-400'
                : 'text-slate-500 hover:text-slate-300'
              }`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Tab 内容 */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-6">
        {tab === 'search' && <SearchTab />}
        {tab === 'upload' && <UploadTab />}
        {tab === 'text' && <TextIngestTab />}
      </div>
    </main>
  );
}
