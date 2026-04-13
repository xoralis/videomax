import { Play } from 'lucide-react';
import { useState } from 'react';
import { cn } from '../lib/utils';
import { createVideoTask } from '../services/api';
import ImageDropzone from './ImageDropzone';

export default function CreateForm({ onTaskCreated }) {
  const [idea, setIdea] = useState('');
  const [images, setImages] = useState([]);
  const [aspectRatio, setAspectRatio] = useState('16:9');
  const [model, setModel] = useState('doubao-seedance-1-0-pro-250528');
  const [duration, setDuration] = useState(5);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');

  const aspectRatios = ['16:9', '9:16', '1:1', '4:3', '3:4'];

  const models = [
    { value: 'doubao-seedance-1-0-pro-250528', label: 'Doubao Seedance Pro' },
    { value: 'kling-v1-6', label: 'Kling v1.6' },
    { value: 'hunyuan-video', label: 'Hunyuan Video' },
  ];

  const durations = [5, 10];

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!idea.trim() && images.length === 0) {
      setError('请输入创意描述或上传至少一张图片');
      return;
    }
    
    setIsLoading(true);
    setError('');
    
    try {
      const res = await createVideoTask(idea, images, aspectRatio, model, duration);
      if (res.task_id) {
        onTaskCreated(res.task_id);
      } else {
        throw new Error('未返回任务 ID');
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="w-full max-w-3xl mx-auto space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-700">
      <div className="text-center space-y-4">
        <h1 className="text-4xl md:text-6xl font-extrabold tracking-tight">
          <span className="gradient-text">videoMax</span> Studio
        </h1>
        <p className="text-slate-400 text-lg">AI Multi-Agent Video Generation</p>
      </div>

      <form onSubmit={handleSubmit} className="glass-card rounded-3xl p-6 md:p-10 space-y-8 shadow-2xl relative overflow-hidden">
        {/* 装扮性的背景光效 */}
        <div className="absolute top-0 right-0 -mr-20 -mt-20 w-64 h-64 bg-cyan-500/10 rounded-full blur-[80px] pointer-events-none" />
        <div className="absolute bottom-0 left-0 -ml-20 -mb-20 w-64 h-64 bg-purple-500/10 rounded-full blur-[80px] pointer-events-none" />

        <div className="space-y-3 relative z-10">
          <label className="text-sm font-semibold text-slate-300 uppercase tracking-widest">The Prompt</label>
          <textarea 
            value={idea}
            onChange={(e) => setIdea(e.target.value)}
            placeholder="描述你想生成的视频画面。例如：赛博朋克风格的城市航拍，霓虹灯闪烁，飞行器穿梭..."
            className="w-full h-32 bg-slate-900/50 border border-slate-700 rounded-xl px-4 py-3 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 transition-all resize-none shadow-inner"
          />
        </div>

        <div className="space-y-3 relative z-10">
          <label className="text-sm font-semibold text-slate-300 uppercase tracking-widest">References (Optional)</label>
          <ImageDropzone images={images} setImages={setImages} />
        </div>

        <div className="space-y-3 relative z-10">
          <label className="text-sm font-semibold text-slate-300 uppercase tracking-widest">Aspect Ratio</label>
          <div className="flex flex-wrap gap-3">
            {aspectRatios.map(ratio => (
              <button
                key={ratio}
                type="button"
                onClick={() => setAspectRatio(ratio)}
                className={cn(
                  "px-5 py-2 rounded-full font-medium transition-all duration-300",
                  aspectRatio === ratio 
                    ? "bg-gradient-to-r from-cyan-500 to-blue-500 text-white shadow-[0_0_15px_rgba(0,210,255,0.4)]"
                    : "bg-slate-800 text-slate-400 hover:bg-slate-700 border border-slate-700"
                )}
              >
                {ratio}
              </button>
            ))}
          </div>
        </div>

        <div className="space-y-3 relative z-10">
          <label className="text-sm font-semibold text-slate-300 uppercase tracking-widest">Model</label>
          <div className="flex flex-wrap gap-3">
            {models.map(m => (
              <button
                key={m.value}
                type="button"
                onClick={() => setModel(m.value)}
                className={cn(
                  "px-5 py-2 rounded-full font-medium transition-all duration-300",
                  model === m.value
                    ? "bg-gradient-to-r from-purple-500 to-pink-500 text-white shadow-[0_0_15px_rgba(168,85,247,0.4)]"
                    : "bg-slate-800 text-slate-400 hover:bg-slate-700 border border-slate-700"
                )}
              >
                {m.label}
              </button>
            ))}
          </div>
        </div>

        <div className="space-y-3 relative z-10">
          <label className="text-sm font-semibold text-slate-300 uppercase tracking-widest">Duration</label>
          <div className="flex flex-wrap gap-3">
            {durations.map(d => (
              <button
                key={d}
                type="button"
                onClick={() => setDuration(d)}
                className={cn(
                  "px-5 py-2 rounded-full font-medium transition-all duration-300",
                  duration === d
                    ? "bg-gradient-to-r from-cyan-500 to-blue-500 text-white shadow-[0_0_15px_rgba(0,210,255,0.4)]"
                    : "bg-slate-800 text-slate-400 hover:bg-slate-700 border border-slate-700"
                )}
              >
                {d}s
              </button>
            ))}
          </div>
        </div>

        {error && (
          <div className="relative z-10 bg-red-500/10 border border-red-500/50 text-red-400 px-4 py-3 rounded-xl animate-pulse">
            {error}
          </div>
        )}

        <div className="pt-4 relative z-10">
          <button
            type="submit"
            disabled={isLoading}
            className={cn(
              "w-full rounded-2xl p-[2px] transition-transform duration-300 active:scale-95 group",
              isLoading ? "opacity-70 cursor-not-allowed" : "hover:scale-[1.02]"
            )}
          >
            {/* 边框发光包裹层 */}
            <div className={cn(
              "absolute inset-0 rounded-2xl bg-gradient-to-r from-cyan-400 via-purple-500 to-blue-500",
              isLoading ? "animate-spin" : ""
            )} style={{ opacity: isLoading ? 0.5 : 1, filter: 'blur(4px)' }} />
            
            <div className="relative bg-slate-900 rounded-xl px-4 py-4 flex items-center justify-center gap-3 w-full h-full">
              {isLoading ? (
                <>
                  <div className="w-5 h-5 rounded-full border-2 border-t-cyan-400 border-r-cyan-400 border-b-transparent border-l-transparent animate-spin" />
                  <span className="font-bold text-white tracking-wide">SUMMONING AGENTS...</span>
                </>
              ) : (
                <>
                  <Play className="w-5 h-5 text-cyan-400 group-hover:text-cyan-300 transition-colors" fill="currentColor" />
                  <span className="font-bold text-white tracking-wide text-lg">GENERATE VIDEO</span>
                </>
              )}
            </div>
          </button>
        </div>
      </form>
    </div>
  );
}
