sudo -E  "./cmd/kube-vmtactionsimulator/actionexecutionsimulator" \
	 --v=3 \
	 --master="http://127.0.0.1:8080" \
	 --etcd-servers="http://127.0.0.1:4001" \
	 --pod="frontend-7xcix" \
	 --action="move" \
	 --destination="127.0.0.1">/tmp/kube-vmturbo.log 2>&1