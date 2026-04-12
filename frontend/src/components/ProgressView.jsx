import React, { useEffect, useState, useMemo } from 'react';
import { CheckCircle2, XCircle, Download, ArrowLeft, Loader2, RotateCcw } from 'lucide-react';
import { pollTaskStatus } from '../services/api';
import { cn } from '../lib/utils';
import useSSE from '../hooks/useSSE';

// Agent 流水线定义：名称 + 描述
const AGENT_PIPELINE = [
  { name: 'StoryAgent',      label: '故事策划',   desc: '使用 CoT 范式提炼创意大纲' },
  { name: 'CharacterAgent',  label: '角色设定',   desc: '多模态视觉分析，提取角色锚点' },
  { name: 'StoryboardAgent', label: '分镜规划',   desc: '拆解分镜结构与时间线' },
  { name: 'VisualAgent',     label: '提示词构建', desc: '结合最佳实践改写 Prompt (ReAct)' },
  { name: 'CriticAgent',     label: '质检审核',   desc: '执行严格质检 (Reflection)' },
];

export default function ProgressView({ taskId, onBack }) {
  const [taskStatus, setTaskStatus] = useState('pending');
  const [videoUrl, setVideoUrl] = useState('');
  const [errorMsg, setErrorMsg] = useState('');

  // SSE 实时事件流
  const { events, isConnected } = useSSE(taskId);

  // 根据 SSE 事件计算每个 Agent 的当前状态
  const agentStates = useMemo(() => {
    const states = {};
    AGENT_PIPELINE.forEach(a => { states[a.name] = 'pending'; });

    for (const evt of events) {
      if (evt.agent_name === 'Pipeline') continue; // 跳过全局事件
      if (states[evt.agent_name] !== undefined) {
        if (evt.status === 'running')  states[evt.agent_name] = 'running';
        if (evt.status === 'done')     states[evt.agent_name] = 'done';
        if (evt.status === 'rejected') states[evt.agent_name] = 'rejected';
        if (evt.status === 'error')    states[evt.agent_name] = 'error';
      }
    }
    return states;
  }, [events]);

  // 最新的事件消息（显示在底部）
  const latestMessage = useMemo(() => {
    if (events.length === 0) return '等待 Agent 协作流水线启动...';
    return events[events.length - 1].message;
  }, [events]);

  // 是否有 Pipeline 完成事件（MAS 协作已结束）
  const pipelineDone = useMemo(() => {
    return events.some(e => e.agent_name === 'Pipeline' && e.status === 'done');
  }, [events]);

  // 质检打回次数
  const rejectCount = useMemo(() => {
    return events.filter(e => e.status === 'rejected').length;
  }, [events]);

  // 轮询最终任务状态（视频 URL）
  useEffect(() => {
    let pollInterval;

    const checkStatus = async () => {
      try {
        const res = await pollTaskStatus(taskId);
        if (res.status === 'failed') {
          setTaskStatus('failed');
          setErrorMsg(res.msg || '视频生成失败');
          clearInterval(pollInterval);
        } else if (res.status === 'success' || res.video_url) {
          setTaskStatus('finished');
          setVideoUrl(res.video_url);
          clearInterval(pollInterval);
        } else {
          setTaskStatus(res.status);
        }
      } catch (err) {
        console.error('Polling error:', err);
      }
    };

    checkStatus();
    pollInterval = setInterval(checkStatus, 5000);
    return () => clearInterval(pollInterval);
  }, [taskId]);

  // 计算进度百分比
  const progressPercent = useMemo(() => {
    const doneCount = Object.values(agentStates).filter(s => s === 'done').length;
    const base = (doneCount / AGENT_PIPELINE.length) * 80; // Agent 阶段占 80%
    if (pipelineDone) return 90;
    if (taskStatus === 'finished') return 100;
    return Math.min(base, 80);
  }, [agentStates, pipelineDone, taskStatus]);

  return (
    <div className="w-full max-w-4xl mx-auto animate-in fade-in zoom-in-95 duration-500">
      <div className="glass-card rounded-3xl p-8 shadow-2xl relative overflow-hidden min-h-[600px] flex flex-col">

        {/* 背景光晕 */}
        <div className={cn(
          "absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-96 h-96 rounded-full blur-[100px] pointer-events-none transition-colors duration-1000",
          taskStatus === 'failed' ? "bg-red-500/10" :
          taskStatus === 'finished' ? "bg-green-500/10" : "bg-cyan-500/10"
        )} />

        {/* 顶部导航栏 */}
        <div className="relative z-10 flex items-center justify-between mb-6">
          <button
            onClick={onBack}
            className="flex items-center gap-2 text-slate-400 hover:text-white transition-colors"
          >
            <ArrowLeft className="w-4 h-4" />
            <span>返回创建页</span>
          </button>
          <div className="flex items-center gap-3">
            {isConnected && (
              <span className="flex items-center gap-1.5 text-xs text-emerald-400">
                <span className="w-1.5 h-1.5 bg-emerald-400 rounded-full animate-pulse" />
                SSE 实时连接
              </span>
            )}
            <div className="bg-slate-800/80 px-4 py-1.5 rounded-full border border-slate-700 text-xs font-mono text-cyan-400">
              {taskId.slice(0, 8)}...
            </div>
          </div>
        </div>

        {/* 主内容区 */}
        <div className="relative z-10 flex-1 flex flex-col">

          {/* ===== Agent 协作进度面板（核心） ===== */}
          {taskStatus !== 'finished' && taskStatus !== 'failed' && (
            <div className="flex-1 flex flex-col gap-6">
              <h3 className="text-xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-cyan-400 to-blue-500 text-center">
                MAS Multi-Agent Pipeline
              </h3>

              {/* Agent 步骤条 */}
              <div className="space-y-3">
                {AGENT_PIPELINE.map((agent, idx) => {
                  const state = agentStates[agent.name];
                  return (
                    <div
                      key={agent.name}
                      className={cn(
                        "flex items-center gap-4 p-3 rounded-xl border transition-all duration-500",
                        state === 'running'  && "bg-cyan-500/10 border-cyan-500/30 shadow-lg shadow-cyan-500/5",
                        state === 'done'     && "bg-emerald-500/5 border-emerald-500/20",
                        state === 'rejected' && "bg-amber-500/10 border-amber-500/30",
                        state === 'error'    && "bg-red-500/10 border-red-500/30",
                        state === 'pending'  && "bg-slate-800/30 border-slate-700/50 opacity-50",
                      )}
                    >
                      {/* 状态图标 */}
                      <div className="w-8 h-8 flex items-center justify-center flex-shrink-0">
                        {state === 'pending' && (
                          <span className="text-slate-500 font-mono text-sm">{idx + 1}</span>
                        )}
                        {state === 'running' && (
                          <Loader2 className="w-5 h-5 text-cyan-400 animate-spin" />
                        )}
                        {state === 'done' && (
                          <CheckCircle2 className="w-5 h-5 text-emerald-400" />
                        )}
                        {state === 'rejected' && (
                          <RotateCcw className="w-5 h-5 text-amber-400" />
                        )}
                        {state === 'error' && (
                          <XCircle className="w-5 h-5 text-red-400" />
                        )}
                      </div>

                      {/* Agent 信息 */}
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className={cn(
                            "font-semibold text-sm",
                            state === 'running' ? "text-cyan-300" :
                            state === 'done' ? "text-emerald-300" :
                            state === 'rejected' ? "text-amber-300" :
                            "text-slate-400"
                          )}>
                            {agent.label}
                          </span>
                          <span className="text-xs text-slate-500 font-mono">{agent.name}</span>
                        </div>
                        <p className="text-xs text-slate-500 mt-0.5 truncate">{agent.desc}</p>
                      </div>

                      {/* 状态标签 */}
                      <div className="flex-shrink-0">
                        {state === 'running' && (
                          <span className="text-[10px] px-2 py-0.5 rounded-full bg-cyan-500/20 text-cyan-300 font-medium">
                            执行中
                          </span>
                        )}
                        {state === 'done' && (
                          <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-300 font-medium">
                            完成
                          </span>
                        )}
                        {state === 'rejected' && (
                          <span className="text-[10px] px-2 py-0.5 rounded-full bg-amber-500/20 text-amber-300 font-medium">
                            打回
                          </span>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>

              {/* 质检打回计数 */}
              {rejectCount > 0 && (
                <div className="text-center text-xs text-amber-400/80">
                  质检已打回 {rejectCount} 次，VisualAgent 正在根据反馈优化提示词...
                </div>
              )}

              {/* 进度条 */}
              <div className="w-full bg-slate-800 h-1.5 rounded-full overflow-hidden">
                <div
                  className="h-full bg-gradient-to-r from-cyan-400 to-purple-500 transition-all duration-700 ease-out"
                  style={{ width: `${progressPercent}%` }}
                />
              </div>

              {/* 最新消息 */}
              <div className="text-center">
                <p className="text-sm text-slate-300 animate-in fade-in duration-300" key={latestMessage}>
                  {latestMessage}
                </p>
              </div>

              {/* 视频生成阶段 */}
              {pipelineDone && (
                <div className="text-center animate-in fade-in slide-in-from-bottom-2 duration-500">
                  <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-purple-500/10 border border-purple-500/20">
                    <Loader2 className="w-4 h-4 text-purple-400 animate-spin" />
                    <span className="text-sm text-purple-300">视频渲染中，请耐心等待...</span>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* ===== 失败状态 ===== */}
          {taskStatus === 'failed' && (
            <div className="flex-1 flex flex-col items-center justify-center gap-4 text-center">
              <XCircle className="w-20 h-20 text-red-500" />
              <h3 className="text-2xl font-bold text-white">Generation Failed</h3>
              <p className="text-slate-400">{errorMsg || '系统内部错误或提供商接口返回失败'}</p>
            </div>
          )}

          {/* ===== 成功状态 ===== */}
          {taskStatus === 'finished' && (
            <div className="flex-1 flex justify-center items-center">
              <div className="w-full max-w-3xl rounded-xl overflow-hidden border border-slate-700 shadow-2xl relative group bg-black">
                <video
                  src={videoUrl}
                  controls
                  autoPlay
                  loop
                  className="w-full max-h-[60vh] object-contain"
                />
                {videoUrl && (
                  <a
                    href={videoUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="absolute top-4 right-4 bg-black/60 hover:bg-black p-3 translate-y-[-10px] opacity-0 group-hover:opacity-100 group-hover:translate-y-0 transition-all duration-300 rounded-full backdrop-blur-sm"
                  >
                    <Download className="w-5 h-5 text-white" />
                  </a>
                )}
              </div>
            </div>
          )}

        </div>
      </div>
    </div>
  );
}
