import{a as h}from"./index-KrCQm35G.js";import{d as v,o as x,c as a,a as t,F as y,r as m,t as r,e as c,f as o,n as f}from"./index-IbMqbDGb.js";const b={class:"space-y-4"},k={key:0,class:"text-gray-500"},z={key:1,class:"grid gap-4 lg:grid-cols-2"},$={class:"rounded-lg border border-gray-800 bg-gray-900 divide-y divide-gray-800"},w=["onClick"],F={class:"text-sm"},S={class:"min-w-0 flex-1"},B={class:"text-sm text-gray-200 truncate"},C={class:"text-xs text-gray-600 truncate"},D={class:"text-xs text-gray-500"},E={class:"rounded-lg border border-gray-800 bg-gray-900 p-4"},T={key:0},j={class:"flex items-center justify-between mb-3"},L={class:"text-sm font-medium text-gray-200"},M={class:"text-xs text-gray-500"},V={class:"rounded bg-gray-800 p-4 text-xs text-gray-400 whitespace-pre-wrap overflow-auto max-h-[500px]"},O={key:1,class:"flex items-center justify-center py-16 text-gray-600"},A=v({__name:"FilesView",setup(K){const d=c([]),l=c(!0),i=c(null),n=c("");x(async()=>{d.value=await h.listFiles(),l.value=!1});function g(e){e.type!=="dir"&&(i.value=e,e.path.includes(".md")?n.value=`# ${e.name}

This is a mock file content for ${e.path}.

Size: ${e.size} bytes`:e.path.includes(".patch")?n.value=`--- a/internal/auth/session.go
+++ b/internal/auth/session.go
@@ -42,6 +42,8 @@
 func (s *Session) Validate() error {
   if s.Expired() {
     return ErrSessionExpired
+    // TODO: refresh token
+    return s.refreshToken()
   }
   return nil
 }`:e.path.includes(".db")?n.value=`SQLite Database
Size: ${(e.size/1024/1024).toFixed(2)} MB

Tables: projects, tasks, task_dependencies, task_attempts, task_events, tool_calls, observations, test_results, review_results, artifacts`:n.value=`Binary file: ${e.path}
Size: ${e.size} bytes`)}function u(e){return e===0?"-":e<1024?`${e} B`:e<1048576?`${(e/1024).toFixed(1)} KB`:`${(e/1048576).toFixed(2)} MB`}return(e,p)=>(o(),a("div",b,[p[0]||(p[0]=t("div",{class:"rounded-lg border-2 border-dashed border-gray-700 bg-gray-900 p-6 text-center hover:border-gray-600 transition-colors cursor-pointer"},[t("div",{class:"text-gray-500"},[t("span",{class:"text-2xl"},"↑"),t("p",{class:"text-sm mt-1"},"Drop files here or click to upload")])],-1)),l.value?(o(),a("div",k,"Loading...")):(o(),a("div",z,[t("div",$,[(o(!0),a(y,null,m(d.value,s=>{var _;return o(),a("div",{key:s.path,onClick:N=>g(s),class:f(["flex items-center gap-3 px-4 py-2 cursor-pointer transition-colors",((_=i.value)==null?void 0:_.path)===s.path?"bg-indigo-900/20":"hover:bg-gray-800/50"])},[t("span",F,r(s.type==="dir"?"📁":"📄"),1),t("div",S,[t("div",B,r(s.name),1),t("div",C,r(s.path),1)]),t("span",D,r(u(s.size)),1)],10,w)}),128))]),t("div",E,[i.value?(o(),a("div",T,[t("div",j,[t("h3",L,r(i.value.path),1),t("span",M,r(u(i.value.size)),1)]),t("pre",V,r(n.value),1)])):(o(),a("div",O," Select a file to preview "))])]))]))}});export{A as default};
