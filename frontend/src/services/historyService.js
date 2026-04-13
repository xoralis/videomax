// services/historyService.js
import { authFetch } from './authService';

/**
 * 获取当前用户的任务历史列表
 * @param {number} page
 * @param {number} pageSize
 */
export async function getTasks(page = 1, pageSize = 10) {
  const res = await authFetch(`/api/tasks?page=${page}&page_size=${pageSize}`);
  if (!res.ok) throw new Error('获取历史记录失败');
  return await res.json();
}

/**
 * 获取当前用户的使用统计
 */
export async function getStats() {
  const res = await authFetch('/api/stats');
  if (!res.ok) throw new Error('获取统计数据失败');
  return await res.json();
}
