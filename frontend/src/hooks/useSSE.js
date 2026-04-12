import { useEffect, useRef, useState, useCallback } from 'react';

/**
 * useSSE - 自定义 Hook，通过 Server-Sent Events 监听 Agent 协作进度
 * @param {string|null} taskId - 任务 ID，为 null 时不连接
 * @returns {{ events: Array, isConnected: boolean }}
 */
export default function useSSE(taskId) {
  const [events, setEvents] = useState([]);
  const [isConnected, setIsConnected] = useState(false);
  const sourceRef = useRef(null);

  const disconnect = useCallback(() => {
    if (sourceRef.current) {
      sourceRef.current.close();
      sourceRef.current = null;
    }
    setIsConnected(false);
  }, []);

  useEffect(() => {
    if (!taskId) return;

    // 清空上一次的事件列表
    setEvents([]);

    const source = new EventSource(`/api/events/${taskId}`);
    sourceRef.current = source;

    source.onopen = () => {
      setIsConnected(true);
    };

    // 监听 "agent" 类型的事件（对应后端的 event: agent）
    source.addEventListener('agent', (e) => {
      try {
        const data = JSON.parse(e.data);
        setEvents(prev => [...prev, data]);
      } catch (err) {
        console.error('[SSE] 事件解析失败:', err);
      }
    });

    // 监听 "close" 事件 → 后端主动关闭连接
    source.addEventListener('close', () => {
      disconnect();
    });

    source.onerror = () => {
      // EventSource 会自动重连，但如果服务端已关闭则手动断开
      if (source.readyState === EventSource.CLOSED) {
        disconnect();
      }
    };

    return () => {
      disconnect();
    };
  }, [taskId, disconnect]);

  return { events, isConnected };
}
