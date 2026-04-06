const XLSX = require('xlsx');

const workbook = XLSX.utils.book_new();
const data = [
  ["项目", "内容"],
  ["当前系统时间", "2026-04-14 16:41:13"],
  ["时区", "Local"],
  ["Unix时间戳", "1776156073"],
  ["RFC3339格式", "2026-04-14T16:41:13+08:00"]
];

const worksheet = XLSX.utils.aoa_to_sheet(data);
XLSX.utils.book_append_sheet(workbook, worksheet, "系统时间");

XLSX.writeFile(workbook, process.env.FRONTEND_UPLOAD_DIR + "/system_time.xlsx");
console.log("表格创建成功!");
