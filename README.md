# Once task is done, refresh log data.
        task_info = {}
        response = {
            'id': result.id,
            'name': result.name, # Need set result_extended
            'state': str(result.state),
            'finished': finished,
            'success': not result.failed(),
            'worker': str(worker),
            'log_link': log_link,
            'date_done': None
        }
        standalone = result.name == "tasks.run_standalone"
        finished = False
        if result.ready():
            try:
                task_info = result.get()
                # Only standalone task has gr result.
                if task_info:
                    response['node'] = task_info.get('node')
                    response['testdir'] = task_info.get('testdir')
                    response['receivers'] = task_info.get('receivers')
                    response['cert_type'] = task_info.get('cert_type')
                    response['enable_spf'] = task_info.get('enable_spf')
                    response['gr_result'] = task_info.get('gr_result', 'unknown')
                    response['gr_status'] = task_info.get('gr_status', {})
                    response['finished'] = True
                    start = task_info['started_at']
                    end = task_info['finished_at']
                    worker = task_info['worker']
            except Exception as e:
                logger.exception(e)
                finished = True
            # Only standalone task has result
        log_link = compose_log_link(start, end=end, podname=worker)
        # Update the data_done once task finished.
        if result.date_done:
            response['date_done'] = result.date_done.strftime('%Y-%m-%dT%H:%M:%S.%fZ')
        return Response(response)