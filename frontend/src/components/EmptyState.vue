<script setup>
// Empty state component - no logic needed
</script>

<template>
  <div class="empty-state">
    <!-- Cat avatar -->
    <div class="cat-avatar-container">
      <img src="/cat_avatar.png" alt="cat" class="cat-avatar" />
      <div class="cat-name-badge">小午</div>
    </div>

    <h2>有什么可以帮你的,主人?</h2>
    <p>小午在这里为你服务喵~ 输入问题开始对话,支持文件上传和图片理解</p>
  </div>
</template>

<style scoped>
.cat-avatar-container {
  position: relative;
  display: inline-block;
  margin-bottom: 20px;
}

.cat-avatar {
  width: 100px;
  height: 100px;
  object-fit: cover;
  border-radius: 50%;
}

.cat-name-badge {
  position: absolute;
  bottom: -5px;
  left: 50%;
  transform: translateX(-50%);
  background: linear-gradient(135deg, #FFB74D, #FF8A65);
  color: #fff;
  font-size: 12px;
  font-weight: bold;
  padding: 2px 10px;
  border-radius: 10px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.2);
}
</style>
