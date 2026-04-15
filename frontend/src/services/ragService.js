// services/ragService.js
import { authFetch } from './authService';

/**
 * 语义检索
 * @param {string} query
 * @param {number} topK
 */
export async function searchKnowledge(query, topK = 3) {
  const params = new URLSearchParams({ query, top_k: topK });
  const res = await authFetch(`/api/rag/search?${params}`);
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.msg || '检索失败');
  }
  return await res.json(); // { code, msg, results: [{id, content, metadata}] }
}

/**
 * 上传文件入库（.txt / .md / .pdf）
 * @param {File} file
 * @param {string} source  可选来源标签
 */
export async function ingestFile(file, source = '') {
  const formData = new FormData();
  formData.append('file', file);
  if (source) formData.append('source', source);

  const res = await authFetch('/api/rag/ingest/file', {
    method: 'POST',
    body: formData,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.msg || '文件入库失败');
  }
  return await res.json(); // { code, msg, ingested }
}

/**
 * 直接提交文本入库
 * @param {{ id: string, content: string, metadata?: object }[]} documents
 */
export async function ingestText(documents) {
  const res = await authFetch('/api/rag/ingest/text', {
    method: 'POST',
    body: JSON.stringify({ documents }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.msg || '文本入库失败');
  }
  return await res.json();
}
