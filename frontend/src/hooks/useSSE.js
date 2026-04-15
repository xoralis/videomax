import { fetchEventSource } from '@microsoft/fetch-event-source';
import { useEffect, useRef, useState } from 'react';
import { getToken } from '../services/authService';

/**
 * useSSE - 自定义 Hook，通过 Server-Sent Events 监听 Agent 协作进度
 * 使用 fetch-event-source 以支持在请求头中传递 JWT，避免 token 暴露在 URL 中
 * @param {string|null} taskId - 任务 ID，为 null 时不连接
 * @returns {{ events: Array, isConnected: boolean }}
 */
export default function useSSE(taskId) {
  const [events, setEvents] = useState([]);
  const [isConnected, setIsConnected] = useState(false);
  const abortCtrlRef = useRef(null);

  useEffect(() => {
    if (!taskId) return;

    // 清空上一次的事件列表
    setEvents([]);

    const ctrl = new AbortController();
    abortCtrlRef.current = ctrl;

    fetchEventSource(`/api/events/${taskId}`, {
      headers: {
        Authorization: `Bearer ${getToken()}`,
      },
      signal: ctrl.signal,
      openWhenHidden: true, // tab 切换/失焦时保持连接，避免断连丢失事件

      onopen(response) {
        if (response.ok) {
          setIsConnected(true);
          return;
        }
        // 非 2xx 响应（如 401）抛出异常，阻止自动重连
        throw new Error(`SSE 连接失败: ${response.status}`);
      },

      onmessage(ev) {
        if (ev.event === 'agent') {
          try {
            const data = JSON.parse(ev.data);
            setEvents(prev => [...prev, data]);
          } catch (err) {
            console.error('[SSE] 事件解析失败:', err);
          }
        } else if (ev.event === 'close') {
          // 后端主动关闭连接
          ctrl.abort();
          setIsConnected(false);
        }
      },

      onerror(err) {
        // 抛出异常以阻止 fetchEventSource 自动重连
        setIsConnected(false);
        throw err;
      },
    }).catch(() => {
      // 捕获 abort 或连接错误，避免 unhandled promise rejection
      setIsConnected(false);
    });

    return () => {
      ctrl.abort();
      setIsConnected(false);
    };
  }, [taskId]);

  return { events, isConnected };
}
