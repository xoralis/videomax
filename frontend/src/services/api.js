// services/api.js

/**
 * 提交视频生成任务
 * @param {string} idea 
 * @param {File[]} images 
 * @param {string} aspectRatio 
 * @param {string} model 视频生成模型，如 doubao-seedance-1-0-pro-250528
 * @param {number} duration 视频时长（秒），5 或 10
 */
export async function createVideoTask(idea, images, aspectRatio, model, duration) {
  const formData = new FormData();
  formData.append('idea', idea);
  formData.append('aspect_ratio', aspectRatio);
  formData.append('model', model);
  formData.append('duration', String(duration));
  
  images.forEach((img) => {
    formData.append('images', img);
  });

  const res = await fetch('/api/video', {
    method: 'POST',
    body: formData,
  });

  if (!res.ok) {
    const errorData = await res.json().catch(() => ({}));
    throw new Error(errorData.msg || '提交任务失败，请重试');
  }

  return await res.json();
}

/**
 * 查询任务状态
 * @param {string} taskId 
 */
export async function pollTaskStatus(taskId) {
  const res = await fetch(`/api/task/${taskId}`);
  
  if (!res.ok) {
    throw new Error('网络异常，查询状态失败');
  }

  return await res.json();
}
