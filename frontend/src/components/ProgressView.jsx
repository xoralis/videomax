import React, { useEffect, useState } from 'react';
import { Loader2, CheckCircle2, XCircle, Download, ArrowLeft } from 'lucide-react';
import { pollTaskStatus } from '../services/api';
import { cn } from '../lib/utils';

export default function ProgressView({ taskId, onBack }) {
  const [status, setStatus] = useState('pending'); // pending, generating, finished, failed
  const [videoUrl, setVideoUrl] = useState('');
  const [errorMsg, setErrorMsg] = useState('');
  
  // 模拟各个 Agent 流水线的动态文本
  const agentStages = [
    "Orchestrator 初始化黑板上下文...",
    "Story Agent 正在使用 CoT 范式提炼创意...",
    "Character Agent 正在解析图片锚点...",
    "Storyboard Agent 拆解分镜结构...",
    "Visual Agent 结合大厂最佳实践改写 Prompt...",
    "Critic Agent 执行严格质检 (Reflection)...",
    "✨ 提示词已放行，提交生成接口...",
    "视频渲染中，请耐心等待 (约需几分钟)..."
  ];
  const [currentStageIdx, setCurrentStageIdx] = useState(0);

  useEffect(() => {
    let stageInterval;
    if (status === 'pending' || status === 'generating') {
      stageInterval = setInterval(() => {
        setCurrentStageIdx(prev => {
          // 在最后一个模拟阶段停住
          if (prev >= agentStages.length - 1) return prev;
          return prev + 1;
        });
      }, 4000);
    }
    return () => clearInterval(stageInterval);
  }, [status, agentStages.length]);

  useEffect(() => {
    let pollInterval;
    
    const checkStatus = async () => {
      try {
        const res = await pollTaskStatus(taskId);
        if (res.status === 'failed') {
          setStatus('failed');
          setErrorMsg('视频生成失败'); 
          clearInterval(pollInterval);
        } else if (res.status === 'finished' || res.video_url) {
          setStatus('finished');
          setVideoUrl(res.video_url || 'https://example.com/demo.mp4'); // Fallback for stub
          clearInterval(pollInterval);
        } else {
          setStatus(res.status);
        }
      } catch (err) {
        console.error("Polling error:", err);
      }
    };

    // 立即查一次
    checkStatus();
    // 5秒查一次
    pollInterval = setInterval(checkStatus, 5000);

    return () => clearInterval(pollInterval);
  }, [taskId]);

  return (
    <div className="w-full max-w-4xl mx-auto animate-in fade-in zoom-in-95 duration-500">
      <div className="glass-card rounded-3xl p-8 shadow-2xl relative overflow-hidden min-h-[500px] flex flex-col">
        
        {/* 背景光晕 */}
        <div className={cn(
          "absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-96 h-96 rounded-full blur-[100px] pointer-events-none transition-colors duration-1000",
          status === 'failed' ? "bg-red-500/10" : 
          status === 'finished' ? "bg-green-500/10" : "bg-cyan-500/10"
        )} />

        <div className="relative z-10 flex items-center justify-between mb-8">
          <button 
            onClick={onBack}
            className="flex items-center gap-2 text-slate-400 hover:text-white transition-colors"
          >
            <ArrowLeft className="w-4 h-4" />
            <span>返回创建页</span>
          </button>
          <div className="bg-slate-800/80 px-4 py-1.5 rounded-full border border-slate-700 text-xs font-mono text-cyan-400">
            TaskID: {taskId}
          </div>
        </div>

        <div className="relative z-10 flex-1 flex flex-col items-center justify-center">
          
          {(status === 'pending' || status === 'generating') && (
            <div className="flex flex-col items-center gap-8 w-full max-w-md">
              <div className="relative">
                <div className="w-24 h-24 rounded-full border-4 border-slate-800 flex items-center justify-center">
                  <div className="w-16 h-16 bg-gradient-to-tr from-cyan-400 to-purple-500 rounded-full animate-bounce" style={{ animationDuration: '2s' }} />
                </div>
                <div className="absolute inset-0 rounded-full border-t-4 border-cyan-400 animate-spin" style={{ animationDuration: '3s' }} />
                <div className="absolute inset-0 rounded-full border-b-4 border-purple-500 animate-spin" style={{ animationDuration: '2s', animationDirection: 'reverse' }} />
              </div>

              <div className="space-y-2 text-center w-full">
                <h3 className="text-xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-cyan-400 to-blue-500">
                  MAS Pipeline Processing
                </h3>
                <div className="h-12 flex items-center justify-center overflow-hidden">
                  <p key={currentStageIdx} className="text-slate-300 animate-in slide-in-from-bottom-2 fade-in duration-300">
                    {agentStages[currentStageIdx]}
                  </p>
                </div>
                <div className="w-full bg-slate-800 h-1 mt-4 rounded-full overflow-hidden">
                  <div 
                    className="h-full bg-gradient-to-r from-cyan-400 to-purple-500 transition-all duration-1000 ease-linear"
                    style={{ width: `${Math.min(((currentStageIdx + 1) / agentStages.length) * 100, 95)}%` }}
                  />
                </div>
              </div>
            </div>
          )}

          {status === 'failed' && (
            <div className="flex flex-col items-center gap-4 text-center">
              <XCircle className="w-20 h-20 text-red-500" />
              <h3 className="text-2xl font-bold text-white">Generation Failed</h3>
              <p className="text-slate-400">{errorMsg || "系统内部错误或提供商接口返回失败"}</p>
            </div>
          )}

          {status === 'finished' && (
            <div className="w-full flex justify-center items-center h-full max-w-3xl">
              <div className="w-full rounded-xl overflow-hidden border border-slate-700 shadow-2xl relative group bg-black">
                {/* 如果后端 API 返回的是 .mp4 地址，用 video 标签兜底播放 */}
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
