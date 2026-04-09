import React, { useCallback, useState } from 'react';
import { UploadCloud, X } from 'lucide-react';
import { cn } from '../lib/utils';

export default function ImageDropzone({ images, setImages }) {
  const [isDragActive, setIsDragActive] = useState(false);

  const handleDragEnter = useCallback((e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragActive(true);
  }, []);

  const handleDragLeave = useCallback((e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragActive(false);
  }, []);

  const handleDrop = useCallback((e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragActive(false);

    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      const droppedFiles = Array.from(e.dataTransfer.files).filter(file => file.type.startsWith('image/'));
      setImages(prev => [...prev, ...droppedFiles]);
    }
  }, [setImages]);

  const handleChange = (e) => {
    if (e.target.files && e.target.files.length > 0) {
      const selectedFiles = Array.from(e.target.files);
      setImages(prev => [...prev, ...selectedFiles]);
    }
  };

  const removeImage = (index) => {
    setImages(prev => prev.filter((_, i) => i !== index));
  };

  return (
    <div className="w-full space-y-4">
      <div 
        className={cn(
          "w-full rounded-2xl glass-card border-dashed border-2 flex flex-col items-center justify-center p-8 cursor-pointer transition-all duration-300 relative",
          isDragActive ? "border-cyan-400 bg-cyan-900/20" : "border-slate-600 hover:border-slate-400 hover:bg-slate-800/40"
        )}
        onDragEnter={handleDragEnter}
        onDragOver={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <input 
          type="file" 
          multiple 
          accept="image/*" 
          className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
          onChange={handleChange}
        />
        <div className="flex flex-col items-center gap-3 pointer-events-none">
          <UploadCloud className={cn("w-10 h-10 transition-colors duration-300", isDragActive ? "text-cyan-400" : "text-slate-400")} />
          <p className="text-slate-300 text-center font-medium">
            {isDragActive ? "释放即可添加图片" : "拖拽参考图片到此处，或点击上传"}
          </p>
          <p className="text-xs text-slate-500">支持 JPG, PNG格式，可选</p>
        </div>
      </div>

      {images.length > 0 && (
        <div className="flex flex-wrap gap-4 mt-4">
          {images.map((file, i) => {
            const objectUrl = URL.createObjectURL(file);
            return (
              <div key={`${file.name}-${i}`} className="relative group w-24 h-24 rounded-lg overflow-hidden border border-slate-700 shadow-lg">
                <img src={objectUrl} alt="preview" className="w-full h-full object-cover" />
                <button 
                  onClick={() => removeImage(i)}
                  className="absolute inset-0 bg-black/60 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-200"
                >
                  <X className="w-6 h-6 text-white" />
                </button>
              </div>
            )
          })}
        </div>
      )}
    </div>
  );
}
