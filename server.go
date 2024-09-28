// Copyright 2009 The Go Authors. All rights reserved.
// 使用此源代码受 BSD 风格的许可
// 许可证可以在 LICENSE 文件中找到。

package Go_rpc

import (
	"Go-rpc/codec"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

const MagicNumber = 0x3bef5c // 定义魔数

// Option 结构体包含 RPC 选项
type Option struct {
	MagicNumber int        // MagicNumber 用于标识这是一个 Gorpc 请求
	CodecType   codec.Type // 客户端可以选择不同的编码器来编码主体
}

// 默认选项
var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

// Server 表示一个 RPC 服务器
type Server struct{}

// NewServer 返回一个新的 Server 实例
func NewServer() *Server {
	return &Server{}
}

// DefaultServer 是默认的 *Server 实例
var DefaultServer = NewServer()

// ServeConn 在单个连接上运行服务器。
// ServeConn 会阻塞，直到客户端断开连接
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }() // 确保在结束时关闭连接
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil { // 解码选项
		log.Println("rpc server: options error: ", err)
		return
	}
	if opt.MagicNumber != MagicNumber { // 验证魔数
		log.Printf("rpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType] // 根据 CodecType 获取编码器
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	server.serveCodec(f(conn)) // 使用选定的编码器处理连接
}

// invalidRequest 是一个占位符，用于响应 argv 时发生错误
var invalidRequest = struct{}{}

// serveCodec 处理编码器
func (server *Server) serveCodec(cc codec.Codec) {
	sending := new(sync.Mutex) // 确保发送完整响应
	wg := new(sync.WaitGroup)  // 等待所有请求处理完成
	for {
		req, err := server.readRequest(cc) // 读取请求
		if err != nil {
			if req == nil {
				break // 无法恢复，关闭连接
			}
			req.h.Error = err.Error()                               // 设置错误信息
			server.sendResponse(cc, req.h, invalidRequest, sending) // 发送响应
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg) // 处理请求
	}
	wg.Wait()      // 等待所有处理完成
	_ = cc.Close() // 关闭编码器
}

// request 存储调用的所有信息
type request struct {
	h            *codec.Header // 请求头
	argv, replyv reflect.Value // 请求参数和响应值
}

// readRequestHeader 读取请求头
func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil { // 读取头部信息
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

// readRequest 读取请求
func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc) // 读取请求头
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	// TODO: 目前我们不知道请求参数的类型
	// 第一天，假设它是字符串
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = cc.ReadBody(req.argv.Interface()); err != nil { // 读取请求体
		log.Println("rpc server: read argv err:", err)
	}
	return req, nil
}

// sendResponse 发送响应
func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()                    // 释放锁
	if err := cc.Write(h, body); err != nil { // 写入响应
		log.Println("rpc server: write response error:", err)
	}
}

// handleRequest 处理请求
func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	// TODO: 应该调用注册的 RPC 方法以获取正确的响应
	// 第一天，只打印 argv 并发送一个 hello 消息
	defer wg.Done() // 完成后减少计数
	log.Println(req.h, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.h.Seq)) // 生成响应
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)        // 发送响应
}

// Accept 在监听器上接受连接并处理请求
// 为每个传入的连接提供服务
func (server *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept() // 接受连接
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go server.ServeConn(conn) // 并发处理连接
	}
}

// Accept 在监听器上接受连接并处理请求
func Accept(lis net.Listener) { DefaultServer.Accept(lis) }
